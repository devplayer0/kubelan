package kubelan

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"

	log "github.com/sirupsen/logrus"
	"k8s.io/api/discovery/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

func metaShortName(meta *v1.ObjectMeta) string {
	return fmt.Sprintf("%v/%v", meta.Namespace, meta.Name)
}

// Manager watches for changes in services and sets up VXLAN FDB's
type Manager struct {
	k8s    *kubernetes.Clientset
	config Config

	services map[string]struct{}
	stop     chan struct{}
	vxlan    *VXLAN

	hookCtx    context.Context
	hookCancel context.CancelFunc
}

func (m *Manager) changed(deleted bool, eps *v1beta1.EndpointSlice) {
	var svc string
	for _, ref := range eps.GetOwnerReferences() {
		if ref.Kind != "Service" {
			continue
		}

		svc = fmt.Sprintf("%v/%v", eps.GetNamespace(), ref.Name)
		break
	}
	if svc == "" {
		log.WithField("EndpointSlice", metaShortName(&eps.ObjectMeta)).Debug("No service found for EndpointSlice")
		return
	}
	if _, ok := m.services[svc]; !ok {
		// Not one of ours
		return
	}

	log.WithFields(log.Fields{
		"Service": svc,
		"deleted": deleted,
	}).Info("Service endpoints changed!")

	ips := []net.IP{}
	for _, ep := range eps.Endpoints {
		for _, addr := range ep.Addresses {
			ip := net.ParseIP(addr)
			if ip == nil {
				log.WithFields(log.Fields{
					"Service": svc,
					"ip":      ip,
				}).Warn("Failed to parse endpoint IP")
				continue
			}
			if ip.Equal(m.config.IP) {
				continue
			}

			ips = append(ips, ip)
		}
	}

	log.WithFields(log.Fields{
		"Service": svc,
		"ips":     ips,
	}).Debug("Service IP addresses")
	for _, ip := range ips {
		if deleted {
			if err := m.vxlan.RemovePeer(ip); err != nil {
				log.WithFields(log.Fields{
					"Service": svc,
					"ip":      ip,
				}).WithError(err).Warn("Failed to remove peer")
			}
		} else {
			if err := m.vxlan.AddPeer(ip); err != nil {
				log.WithFields(log.Fields{
					"Service": svc,
					"ip":      ip,
				}).WithError(err).Warn("Failed to add peer")
			}
		}
	}

	if len(m.config.Hooks.Change) > 0 {
		sIPs := make([]string, len(ips))
		for i, ip := range ips {
			sIPs[i] = ip.String()
		}

		cmd := exec.CommandContext(m.hookCtx, m.config.Hooks.Change[0], m.config.Hooks.Change[1:]...)
		cmd.Env = append(os.Environ(),
			"IFACE="+m.config.VXLAN.Interface,
			"SERVICE="+svc,
			"IPS="+strings.Join(sIPs, " "),
			fmt.Sprintf("DELETED=%v", deleted),
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		go func() {
			if err := cmd.Run(); err != nil {
				log.WithError(err).Warn("Change hook failed")
			}
		}()
	}
}

// NewManager creates a new manager
func NewManager(k8sConf *rest.Config, config Config) (*Manager, error) {
	k8s, err := kubernetes.NewForConfig(k8sConf)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client")
	}

	services := make(map[string]struct{})
	for _, service := range config.Services {
		if service.Namespace == "" {
			if config.Namespace == "" {
				log.WithField("Service", service.Name).Warn("Default namespace unset, skipping service without namespace")
				continue
			}

			service.Namespace = config.Namespace
		}

		services[metaShortName(&service)] = struct{}{}
	}

	hookCtx, hookCancel := context.WithCancel(context.Background())
	return &Manager{
		k8s,
		config,

		services,
		make(chan struct{}),
		nil,
		hookCtx,
		hookCancel,
	}, nil
}

// Start starts watching services
func (m *Manager) Start() error {
	log.Info("Starting kubelan manager")

	var err error
	m.vxlan, err = NewVXLAN(m.config.VXLAN.Interface, m.config.VXLAN.MTU, m.config.VXLAN.VNI, m.config.IP, m.config.VXLAN.Port)
	if err != nil {
		return fmt.Errorf("failed to create VXLAN interface: %w", err)
	}

	if len(m.config.Hooks.Up) > 0 {
		cmd := exec.CommandContext(m.hookCtx, m.config.Hooks.Up[0], m.config.Hooks.Up[1:]...)
		cmd.Env = append(os.Environ(),
			"IFACE="+m.config.VXLAN.Interface,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		go func() {
			if err := cmd.Run(); err != nil {
				log.WithError(err).Warn("Up hook failed")
			}
		}()
	}

	factory := informers.NewSharedInformerFactory(m.k8s, 0)
	factory.Discovery().V1beta1().EndpointSlices().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			log.WithField("EndpointSlice", obj).Trace("EndpointSlice added")
			m.changed(false, obj.(*v1beta1.EndpointSlice))
		},
		DeleteFunc: func(obj interface{}) {
			log.WithField("EndpointSlice", obj).Trace("EndpointSlice deleted")
			m.changed(true, obj.(*v1beta1.EndpointSlice))
		},
		UpdateFunc: func(old, new interface{}) {
			log.WithField("EndpointSlice", new).Trace("EndpointSlice updated")
			m.changed(true, old.(*v1beta1.EndpointSlice))
			m.changed(false, new.(*v1beta1.EndpointSlice))
		},
	})

	factory.Start(m.stop)
	return nil
}

// Stop stops watching services
func (m *Manager) Stop() error {
	log.Info("Stopping kubelan manager")

	close(m.stop)

	m.hookCancel()

	if err := m.vxlan.Delete(); err != nil {
		return fmt.Errorf("failed to delete VXLAN interface: %w", err)
	}

	return nil
}
