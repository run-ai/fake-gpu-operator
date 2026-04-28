package mock

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

// Reconciler runs the desired-state pipeline for mock-pool resources.
type Reconciler struct {
	kube   kubernetes.Interface
	params ReconcileParams
}

// NewReconciler constructs a Reconciler. ReconcileParams.Namespace must be
// the namespace of the topology ConfigMap (and where per-pool resources
// are created).
func NewReconciler(kube kubernetes.Interface, params ReconcileParams) *Reconciler {
	return &Reconciler{kube: kube, params: params}
}

// Reconcile performs one full pipeline pass: read topology, compute desired
// state, diff against actual, apply.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	cfg, err := r.readTopologyConfig(ctx)
	if err != nil {
		return fmt.Errorf("reading topology config: %w", err)
	}

	desired, err := ComputeDesiredState(r.kube, cfg, r.params)
	if err != nil {
		return fmt.Errorf("computing desired state: %w", err)
	}

	if err := r.reconcileConfigMaps(ctx, desired); err != nil {
		return fmt.Errorf("reconciling ConfigMaps: %w", err)
	}
	if err := r.reconcileDaemonSets(ctx, desired); err != nil {
		return fmt.Errorf("reconciling DaemonSets: %w", err)
	}
	return nil
}

func (r *Reconciler) readTopologyConfig(ctx context.Context) (*topology.ClusterConfig, error) {
	cm, err := r.kube.CoreV1().ConfigMaps(r.params.Namespace).
		Get(ctx, "topology", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return topology.FromClusterConfigCM(cm)
}

// managedSelector matches only the per-pool resources this controller owns —
// keeps List() blind to anything Helm or another controller created.
func (r *Reconciler) managedSelector() string {
	return constants.LabelManagedBy + "=" + constants.LabelManagedByValue +
		"," + constants.LabelComponent + "=" + constants.ComponentNvmlMock
}

func (r *Reconciler) reconcileDaemonSets(ctx context.Context, desired []runtime.Object) error {
	actual, err := r.kube.AppsV1().DaemonSets(r.params.Namespace).
		List(ctx, metav1.ListOptions{LabelSelector: r.managedSelector()})
	if err != nil {
		return fmt.Errorf("listing managed DaemonSets: %w", err)
	}
	d := DiffDaemonSets(desired, actual.Items)

	for _, ds := range d.ToCreate {
		log.Printf("Creating DaemonSet %s/%s", ds.Namespace, ds.Name)
		if _, err := r.kube.AppsV1().DaemonSets(r.params.Namespace).
			Create(ctx, ds, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating DaemonSet %s: %w", ds.Name, err)
		}
	}
	for _, ds := range d.ToUpdate {
		log.Printf("Updating DaemonSet %s/%s", ds.Namespace, ds.Name)
		if _, err := r.kube.AppsV1().DaemonSets(r.params.Namespace).
			Update(ctx, ds, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("updating DaemonSet %s: %w", ds.Name, err)
		}
	}
	for _, ds := range d.ToDelete {
		log.Printf("Deleting DaemonSet %s/%s", ds.Namespace, ds.Name)
		if err := r.kube.AppsV1().DaemonSets(r.params.Namespace).
			Delete(ctx, ds.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("deleting DaemonSet %s: %w", ds.Name, err)
		}
	}
	return nil
}

func (r *Reconciler) reconcileConfigMaps(ctx context.Context, desired []runtime.Object) error {
	actual, err := r.kube.CoreV1().ConfigMaps(r.params.Namespace).
		List(ctx, metav1.ListOptions{LabelSelector: r.managedSelector()})
	if err != nil {
		return fmt.Errorf("listing managed ConfigMaps: %w", err)
	}
	d := DiffConfigMaps(desired, actual.Items)

	for _, cm := range d.ToCreate {
		log.Printf("Creating ConfigMap %s/%s", cm.Namespace, cm.Name)
		if _, err := r.kube.CoreV1().ConfigMaps(r.params.Namespace).
			Create(ctx, cm, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating ConfigMap %s: %w", cm.Name, err)
		}
	}
	for _, cm := range d.ToUpdate {
		log.Printf("Updating ConfigMap %s/%s", cm.Namespace, cm.Name)
		if _, err := r.kube.CoreV1().ConfigMaps(r.params.Namespace).
			Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("updating ConfigMap %s: %w", cm.Name, err)
		}
	}
	for _, cm := range d.ToDelete {
		log.Printf("Deleting ConfigMap %s/%s", cm.Namespace, cm.Name)
		if err := r.kube.CoreV1().ConfigMaps(r.params.Namespace).
			Delete(ctx, cm.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("deleting ConfigMap %s: %w", cm.Name, err)
		}
	}
	return nil
}
