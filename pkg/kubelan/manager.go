package kubelan

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

// Manager watches for changes in services and sets up VXLAN FDB's
type Manager struct {
	k8s  *kubernetes.Clientset
	stop chan struct{}
}

// NewManager creates a new manager
func NewManager(config *rest.Config) (*Manager, error) {
	k8s, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client")
	}

	return &Manager{
		k8s,
		make(chan struct{}),
	}, nil
}

// Start starts watching services
func (m *Manager) Start() {
	factory := informers.NewSharedInformerFactory(m.k8s, 30*time.Second)
	factory.Core().V1().Services().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(svc interface{}) {
			log.WithField("service", svc).Trace("Service added")
		},
		DeleteFunc: func(svc interface{}) {
			log.WithField("service", svc).Trace("Service deleted")
		},
		UpdateFunc: func(old, new interface{}) {
			log.WithField("service", new).Trace("Service updated")
		},
	})

	factory.Start(m.stop)
}

// Stop stops watching services
func (m *Manager) Stop() {
	close(m.stop)
}
