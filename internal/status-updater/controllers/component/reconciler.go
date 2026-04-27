package component

import (
	"context"
	"fmt"
	"log"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

// Reconciler manages the lifecycle of per-pool component resources.
type Reconciler struct {
	kubeClient kubernetes.Interface
	params     ReconcileParams
}

func NewReconciler(kubeClient kubernetes.Interface, params ReconcileParams) *Reconciler {
	return &Reconciler{
		kubeClient: kubeClient,
		params:     params,
	}
}

// Reconcile reads the topology CM, computes desired state, diffs against actual, and applies changes.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	config, err := r.readConfig(ctx)
	if err != nil {
		return fmt.Errorf("reading topology config: %w", err)
	}

	desired := ComputeDesiredState(config, r.params)

	if err := r.reconcileDeployments(ctx, desired); err != nil {
		return fmt.Errorf("reconciling deployments: %w", err)
	}

	if err := r.reconcileServices(ctx, desired); err != nil {
		return fmt.Errorf("reconciling services: %w", err)
	}

	return nil
}

func (r *Reconciler) readConfig(ctx context.Context) (*topology.ClusterConfig, error) {
	cm, err := r.kubeClient.CoreV1().ConfigMaps(r.params.Namespace).Get(ctx, "topology", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return topology.FromClusterConfigCM(cm)
}

func (r *Reconciler) reconcileDeployments(ctx context.Context, desired []runtime.Object) error {
	managedSelector := constants.LabelManagedBy + "=" + constants.LabelManagedByValue

	actualList, err := r.kubeClient.AppsV1().Deployments(r.params.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: managedSelector,
	})
	if err != nil {
		return fmt.Errorf("listing managed deployments: %w", err)
	}

	diff := DiffDeployments(desired, actualList.Items)

	for _, dep := range diff.ToCreate {
		log.Printf("Creating deployment %s", dep.Name)
		if _, err := r.kubeClient.AppsV1().Deployments(r.params.Namespace).Create(ctx, dep, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating deployment %s: %w", dep.Name, err)
		}
	}

	for _, dep := range diff.ToUpdate {
		log.Printf("Updating deployment %s", dep.Name)
		if _, err := r.kubeClient.AppsV1().Deployments(r.params.Namespace).Update(ctx, dep, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("updating deployment %s: %w", dep.Name, err)
		}
	}

	for _, dep := range diff.ToDelete {
		log.Printf("Deleting deployment %s", dep.Name)
		if err := r.kubeClient.AppsV1().Deployments(r.params.Namespace).Delete(ctx, dep.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("deleting deployment %s: %w", dep.Name, err)
		}
	}

	return nil
}

func (r *Reconciler) reconcileServices(ctx context.Context, desired []runtime.Object) error {
	managedSelector := constants.LabelManagedBy + "=" + constants.LabelManagedByValue

	actualList, err := r.kubeClient.CoreV1().Services(r.params.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: managedSelector,
	})
	if err != nil {
		return fmt.Errorf("listing managed services: %w", err)
	}

	diff := DiffServices(desired, actualList.Items)

	for _, svc := range diff.ToCreate {
		log.Printf("Creating service %s", svc.Name)
		if _, err := r.kubeClient.CoreV1().Services(r.params.Namespace).Create(ctx, svc, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating service %s: %w", svc.Name, err)
		}
	}

	for _, svc := range diff.ToDelete {
		log.Printf("Deleting service %s", svc.Name)
		if err := r.kubeClient.CoreV1().Services(r.params.Namespace).Delete(ctx, svc.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("deleting service %s: %w", svc.Name, err)
		}
	}

	return nil
}
