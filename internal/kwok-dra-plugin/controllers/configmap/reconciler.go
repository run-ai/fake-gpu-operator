package configmap

import (
	"context"
	"fmt"
	"log"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	rshandler "github.com/run-ai/fake-gpu-operator/internal/kwok-dra-plugin/handlers/resourceslice"
)

// ConfigMapReconciler reconciles ConfigMap objects for KWOK nodes
type ConfigMapReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	handler          *rshandler.ResourceSliceHandler
	namespace        string
	topologyCMPrefix string
}

// Reconcile handles ConfigMap events for KWOK node topology ConfigMaps
func (r *ConfigMapReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var cm corev1.ConfigMap
	if err := r.Get(ctx, req.NamespacedName, &cm); err != nil {
		// ConfigMap was deleted
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		// Handle deletion - extract node name from the ConfigMap name
		nodeName := extractNodeNameFromCMName(req.Name, r.topologyCMPrefix)
		if nodeName != "" {
			if err := r.handler.HandleDelete(nodeName); err != nil {
				log.Printf("Failed to handle ConfigMap deletion for ResourceSlice: %v", err)
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Handle create/update
	if err := r.handler.HandleAddOrUpdate(&cm); err != nil {
		log.Printf("Failed to handle ConfigMap for ResourceSlice: %v", err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// extractNodeNameFromCMName extracts the node name from a topology ConfigMap name
// The ConfigMap name format is: <topology-cm-prefix>-<node-name>
func extractNodeNameFromCMName(cmName, topologyCMPrefix string) string {
	prefix := topologyCMPrefix + "-"
	if strings.HasPrefix(cmName, prefix) {
		return strings.TrimPrefix(cmName, prefix)
	}
	return cmName
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConfigMapReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create a predicate to filter only KWOK node topology ConfigMaps
	kwokNodeCMPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		cm, ok := obj.(*corev1.ConfigMap)
		if !ok {
			return false
		}
		return isFakeGpuKWOKNodeConfigMap(cm)
	})

	// Namespace predicate
	namespacePredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return obj.GetNamespace() == r.namespace
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithEventFilter(predicate.And(namespacePredicate, kwokNodeCMPredicate)).
		Complete(r)
}

// isFakeGpuKWOKNodeConfigMap checks if a ConfigMap is a KWOK node topology ConfigMap
func isFakeGpuKWOKNodeConfigMap(cm *corev1.ConfigMap) bool {
	if cm == nil || cm.Labels == nil || cm.Annotations == nil {
		return false
	}
	_, foundNodeName := cm.Labels[constants.LabelTopologyCMNodeName]
	if !foundNodeName {
		return false
	}

	return cm.Annotations[constants.AnnotationKwokNode] == "fake"
}

// SetupWithManager creates and sets up the ConfigMap reconciler with the manager
func SetupWithManager(mgr ctrl.Manager, kubeClient kubernetes.Interface, namespace, topologyCMName string) error {
	handler := rshandler.NewResourceSliceHandler(kubeClient)

	reconciler := &ConfigMapReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		handler:          handler,
		namespace:        namespace,
		topologyCMPrefix: topologyCMName,
	}

	if err := reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("failed to setup ConfigMap reconciler: %w", err)
	}

	log.Println("ConfigMap reconciler setup complete for KWOK DRA plugin")
	return nil
}
