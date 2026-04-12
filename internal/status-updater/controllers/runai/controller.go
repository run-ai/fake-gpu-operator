package runai

import (
	"context"
	"log"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers"
)

const runaiNamespace = "runai"

type RunaiController struct {
	kubeClient      kubernetes.Interface
	dynamicClient   dynamic.Interface
	pollingInterval time.Duration
	apiAvailable    *bool // cached discovery result; nil = not yet checked
}

var _ controllers.Interface = &RunaiController{}

func NewRunaiController(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, pollingInterval time.Duration) *RunaiController {
	return &RunaiController{
		kubeClient:      kubeClient,
		dynamicClient:   dynamicClient,
		pollingInterval: pollingInterval,
	}
}

func (c *RunaiController) Run(stopCh <-chan struct{}) {
	log.Println("Starting runai controller")

	// Run reconcile immediately on start, then on each tick
	ctx := context.Background()
	if err := c.reconcile(ctx); err != nil {
		log.Printf("runai controller reconcile error: %v", err)
	}

	ticker := time.NewTicker(c.pollingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.reconcile(ctx); err != nil {
				log.Printf("runai controller reconcile error: %v", err)
			}
		case <-stopCh:
			log.Println("Stopping runai controller")
			return
		}
	}
}

func (c *RunaiController) reconcile(ctx context.Context) error {
	// Step 1: Check if monitoring.coreos.com/v1 API is available (cached after first success)
	if !c.isPrometheusAPIAvailable() {
		return nil
	}

	// Step 2: Check if runai namespace exists
	_, err := c.kubeClient.CoreV1().Namespaces().Get(ctx, runaiNamespace, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	// Step 3: Check if PrometheusRule already exists
	_, err = c.dynamicClient.Resource(prometheusRuleGVR).Namespace(runaiNamespace).Get(
		ctx, prometheusRuleName, metav1.GetOptions{})
	if err == nil {
		// Already exists, nothing to do
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}

	// Step 4: Create the PrometheusRule
	rule := buildKwokPrometheusRule(runaiNamespace)
	_, err = c.dynamicClient.Resource(prometheusRuleGVR).Namespace(runaiNamespace).Create(
		ctx, rule, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	log.Printf("Created PrometheusRule %s in namespace %s", prometheusRuleName, runaiNamespace)
	return nil
}

func (c *RunaiController) isPrometheusAPIAvailable() bool {
	if c.apiAvailable != nil {
		return *c.apiAvailable
	}

	_, err := c.kubeClient.Discovery().ServerResourcesForGroupVersion("monitoring.coreos.com/v1")
	if err != nil {
		// API not available -- don't cache so we retry on next tick
		return false
	}

	t := true
	c.apiAvailable = &t
	return true
}
