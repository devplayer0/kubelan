package kubelan

import (
	"fmt"
	"net"

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
	}).Debug("Service endpoints changed!")

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
}

// NewManager creates a new manager
func NewManager(k8sConf *rest.Config, config Config) (*Manager, error) {
	k8s, err := kubernetes.NewForConfig(k8sConf)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client")
	}

	services := make(map[string]struct{})
	for _, service := range config.Services {
		services[metaShortName(&service)] = struct{}{}
	}

	return &Manager{
		k8s,
		config,

		services,
		make(chan struct{}),
	}, nil
}

// Start starts watching services
func (m *Manager) Start() {
	log.Info("Starting kubelan manager")

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
}

// Stop stops watching services
func (m *Manager) Stop() {
	log.Info("Stopping kubelan manager")
	close(m.stop)
}
