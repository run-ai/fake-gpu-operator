package nrt

import (
	"context"

	v1alpha2 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const fieldManager = "fake-gpu-operator"

// nrtGVR identifies the cluster-scoped NodeResourceTopology resource (v1alpha2).
var nrtGVR = schema.GroupVersionResource{
	Group:    "topology.node.k8s.io",
	Version:  "v1alpha2",
	Resource: "noderesourcetopologies",
}

// Publisher applies per-node NodeResourceTopology objects. Deletion is handled by
// Kubernetes garbage collection via the owner reference each NRT carries (its
// owning Node), so there is no Delete here.
type Publisher interface {
	Apply(ctx context.Context, nrt *v1alpha2.NodeResourceTopology) error
}

// Reconciler writes NodeResourceTopology CRs through the dynamic client.
type Reconciler struct {
	client dynamic.Interface
}

var _ Publisher = &Reconciler{}

func NewReconciler(client dynamic.Interface) *Reconciler {
	return &Reconciler{client: client}
}

// Apply server-side-applies the NRT (named after the node). NRT is cluster-scoped.
func (r *Reconciler) Apply(ctx context.Context, nrt *v1alpha2.NodeResourceTopology) error {
	u, err := toUnstructured(nrt)
	if err != nil {
		return err
	}
	_, err = r.client.Resource(nrtGVR).Apply(ctx, nrt.Name, u, metav1.ApplyOptions{
		FieldManager: fieldManager,
		Force:        true,
	})
	return err
}

// toUnstructured converts the typed NRT to unstructured, stamping TypeMeta and
// removing status + null creationTimestamp so the apply touches spec fields only.
func toUnstructured(nrt *v1alpha2.NodeResourceTopology) (*unstructured.Unstructured, error) {
	obj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(nrt)
	if err != nil {
		return nil, err
	}
	delete(obj, "status")
	unstructured.RemoveNestedField(obj, "metadata", "creationTimestamp")
	u := &unstructured.Unstructured{Object: obj}
	u.SetAPIVersion(apiVersion)
	u.SetKind(kind)
	return u, nil
}
