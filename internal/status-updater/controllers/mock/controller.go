package mock

import (
	"context"
	"log"
	"time"

	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// MockController watches the cluster topology ConfigMap and reconciles
// per-pool nvml-mock DaemonSets and ConfigMaps.
type MockController struct {
	informer   cache.SharedIndexInformer
	reconciler *Reconciler
}

var _ controllers.Interface = (*MockController)(nil)

// NewMockController constructs a controller and wires its informer.
// The informer uses a server-side FieldSelector to watch only the topology
// ConfigMap by name, with a 30s resync interval.
func NewMockController(kube kubernetes.Interface, params ReconcileParams) *MockController {
	factory := informers.NewSharedInformerFactoryWithOptions(
		kube,
		30*time.Second,
		informers.WithNamespace(params.Namespace),
		informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.FieldSelector = fields.OneTermEqualSelector("metadata.name", "topology").String()
		}),
	)
	informer := factory.Core().V1().ConfigMaps().Informer()

	c := &MockController{
		informer:   informer,
		reconciler: NewReconciler(kube, params),
	}

	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { c.handleEvent(obj) },
		UpdateFunc: func(_, newObj interface{}) { c.handleEvent(newObj) },
	}); err != nil {
		log.Fatalf("mock controller: failed to add event handler: %v", err)
	}

	return c
}

// Run blocks until stopCh is closed.
func (c *MockController) Run(stopCh <-chan struct{}) {
	log.Println("Starting mock controller")
	c.informer.Run(stopCh)
}

func (c *MockController) handleEvent(obj interface{}) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}
	if cm.Name != "topology" {
		return
	}
	log.Printf("mock controller: topology CM event, reconciling")
	if err := c.reconciler.Reconcile(context.Background()); err != nil {
		log.Printf("mock controller: reconcile error: %v", err)
	}
}
