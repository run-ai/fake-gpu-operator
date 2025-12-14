package dra_plugin_gpu

import (
	"context"
	"fmt"
	"log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// NodeReconciler reconciles Node objects
type NodeReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	state    *DeviceState
	nodeName string
}

// Reconcile watches for changes to the node and refreshes topology from the server
func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if req.Name != r.nodeName {
		return ctrl.Result{}, nil
	}

	var node corev1.Node
	if err := r.Get(ctx, req.NamespacedName, &node); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log.Printf("Node event received, refreshing topology for node %s", req.Name)

	if err := r.state.UpdateDevicesFromTopology(ctx); err != nil {
		log.Printf("Failed to update devices from topology: %v", err)
		return ctrl.Result{}, fmt.Errorf("failed to update devices: %w", err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create a predicate to filter only our node
	nodeNamePredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return obj.GetName() == r.nodeName
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		WithEventFilter(nodeNamePredicate).
		Complete(r)
}

// SetupNodeController sets up and starts the node controller
func SetupNodeController(ctx context.Context, state *DeviceState, nodeName string) error {
	if nodeName == "" {
		return fmt.Errorf("node name is required")
	}

	// Initialize controller-runtime logger using klog
	// This must be called before creating the manager to avoid the warning
	ctrl.SetLogger(klog.NewKlogr())

	// Get controller-runtime config
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}

	// Create scheme
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		return fmt.Errorf("failed to add corev1 to scheme: %w", err)
	}

	// Create manager - we'll filter by node name in the reconciler
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	// Create reconciler
	reconciler := &NodeReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		state:    state,
		nodeName: nodeName,
	}

	// Setup controller
	if err := reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("failed to setup controller: %w", err)
	}

	// Start manager in a goroutine
	go func() {
		if err := mgr.Start(ctx); err != nil {
			log.Printf("Failed to start node controller manager: %v", err)
		}
	}()

	return nil
}
