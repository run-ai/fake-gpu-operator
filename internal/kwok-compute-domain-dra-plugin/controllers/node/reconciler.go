package node

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/pkg/compute-domain/consts"
)

type NodeReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	kubeClient kubernetes.Interface
	namespace  string
}

func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var node corev1.Node
	if err := r.Get(ctx, req.NamespacedName, &node); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
		if err := r.deleteResourceSlice(ctx, req.Name); err != nil {
			klog.ErrorS(err, "Failed to delete ResourceSlice for node", "node", req.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	if err := r.createOrUpdateResourceSlice(ctx, &node); err != nil {
		klog.ErrorS(err, "Failed to create/update ResourceSlice for node", "node", node.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NodeReconciler) resourceSliceName(nodeName string) string {
	return fmt.Sprintf("kwok-%s-compute-domain-channel", nodeName)
}

func (r *NodeReconciler) createOrUpdateResourceSlice(ctx context.Context, node *corev1.Node) error {
	devices := r.enumerateComputeDomainDevices()

	resourceSlice := &resourceapi.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.resourceSliceName(node.Name),
		},
		Spec: resourceapi.ResourceSliceSpec{
			Driver:   consts.ComputeDomainDriverName,
			NodeName: ptr.To(node.Name),
			Pool: resourceapi.ResourcePool{
				Name:               node.Name,
				ResourceSliceCount: 1,
			},
			Devices: devices,
		},
	}

	existing, err := r.kubeClient.ResourceV1().ResourceSlices().Get(ctx, resourceSlice.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = r.kubeClient.ResourceV1().ResourceSlices().Create(ctx, resourceSlice, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create ResourceSlice for node %s: %w", node.Name, err)
			}
			klog.InfoS("Created ResourceSlice for KWOK node", "node", node.Name, "deviceCount", len(devices))
			return nil
		}
		return fmt.Errorf("failed to get ResourceSlice for node %s: %w", node.Name, err)
	}

	resourceSlice.ResourceVersion = existing.ResourceVersion
	_, err = r.kubeClient.ResourceV1().ResourceSlices().Update(ctx, resourceSlice, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update ResourceSlice for node %s: %w", node.Name, err)
	}
	klog.InfoS("Updated ResourceSlice for KWOK node", "node", node.Name, "deviceCount", len(devices))
	return nil
}

func (r *NodeReconciler) deleteResourceSlice(ctx context.Context, nodeName string) error {
	err := r.kubeClient.ResourceV1().ResourceSlices().Delete(ctx, r.resourceSliceName(nodeName), metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ResourceSlice for node %s: %w", nodeName, err)
	}
	klog.InfoS("Deleted ResourceSlice for KWOK node", "node", nodeName)
	return nil
}

func (r *NodeReconciler) enumerateComputeDomainDevices() []resourceapi.Device {
	devices := make([]resourceapi.Device, 0, 1)
	device := r.newChannelDevice(0)
	devices = append(devices, device)
	return devices
}

func (r *NodeReconciler) newChannelDevice(channelID int) resourceapi.Device {
	return resourceapi.Device{
		Name: fmt.Sprintf("channel-%d", channelID),
		Attributes: map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
			"compute-domain.nvidia.com/type": {
				StringValue: ptr.To("channel"),
			},
			"compute-domain.nvidia.com/id": {
				IntValue: ptr.To(int64(channelID)),
			},
		},
	}
}

func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	kwokNodePredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		node, ok := obj.(*corev1.Node)
		if !ok {
			return false
		}
		return isKWOKNode(node)
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		WithEventFilter(kwokNodePredicate).
		Complete(r)
}

func isKWOKNode(node *corev1.Node) bool {
	if node == nil || node.Labels == nil {
		return false
	}
	return node.Labels["type"] == "kwok" || node.Annotations[constants.AnnotationKwokNode] == "fake"
}

func SetupWithManager(mgr ctrl.Manager, kubeClient kubernetes.Interface, namespace string) error {
	reconciler := &NodeReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		kubeClient: kubeClient,
		namespace:  namespace,
	}

	if err := reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("failed to setup Node reconciler: %w", err)
	}

	klog.InfoS("Node reconciler setup complete for KWOK Compute Domain DRA plugin")
	return nil
}
