package component

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

// ComponentController watches the cluster topology ConfigMap and reconciles
// per-pool component Deployments and Services.
type ComponentController struct {
	kubeClient kubernetes.Interface
	informer   cache.SharedIndexInformer
	reconciler *Reconciler
}

var _ controllers.Interface = &ComponentController{}

func NewComponentController(kubeClient kubernetes.Interface, params ReconcileParams) *ComponentController {
	reconciler := NewReconciler(kubeClient, params)

	factory := informers.NewSharedInformerFactoryWithOptions(
		kubeClient,
		30*time.Second,
		informers.WithNamespace(params.Namespace),
		informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.FieldSelector = fields.OneTermEqualSelector("metadata.name", "topology").String()
		}),
	)
	informer := factory.Core().V1().ConfigMaps().Informer()

	c := &ComponentController{
		kubeClient: kubeClient,
		informer:   informer,
		reconciler: reconciler,
	}

	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			c.handleEvent(obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			c.handleEvent(newObj)
		},
	}); err != nil {
		log.Fatalf("Failed to add event handler: %v", err)
	}

	return c
}

func (c *ComponentController) Run(stopCh <-chan struct{}) {
	log.Println("Starting component controller")
	c.informer.Run(stopCh)
}

func (c *ComponentController) handleEvent(obj interface{}) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}
	if cm.Name != "topology" {
		return
	}
	log.Printf("Topology CM changed, reconciling components...")
	if err := c.reconciler.Reconcile(context.Background()); err != nil {
		log.Printf("Error reconciling components: %v", err)
	}
}
