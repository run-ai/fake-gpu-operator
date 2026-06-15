package nrt

import (
	"context"
	"encoding/json"
	"testing"

	v1alpha2 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8stesting "k8s.io/client-go/testing"

	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func sampleNRT(name string) *v1alpha2.NodeResourceTopology {
	return &v1alpha2.NodeResourceTopology{
		TypeMeta:   metav1.TypeMeta{APIVersion: apiVersion, Kind: kind},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Zones:      v1alpha2.ZoneList{{Name: "node-0", Type: "Node"}},
	}
}

func TestToUnstructuredStripsStatusAndStampsTypeMeta(t *testing.T) {
	u, err := toUnstructured(sampleNRT("node-a"))
	require.NoError(t, err)
	assert.Equal(t, apiVersion, u.GetAPIVersion())
	assert.Equal(t, kind, u.GetKind())
	assert.Equal(t, "node-a", u.GetName())
	_, hasStatus := u.Object["status"]
	assert.False(t, hasStatus, "status must be stripped before apply")
	_, hasZones := u.Object["zones"]
	assert.True(t, hasZones, "zones must be present")
	_, hasCreationTS, _ := unstructured.NestedFieldNoCopy(u.Object, "metadata", "creationTimestamp")
	assert.False(t, hasCreationTS, "null creationTimestamp must be removed")
}

func newFakeClient() *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{nrtGVR: "NodeResourceTopologyList"})
}

// applyUpsertReactor is a PrependReactor that handles server-side-apply patch
// actions by upserting the object into the fake tracker. The real fake client's
// tracker.Apply requires the object to pre-exist (it is not an upsert), so
// TestReconcilerApplyCreatesNRT would fail without this shim. This adapter
// lives in the test only — Reconciler.Apply still uses the real .Apply() path.
func applyUpsertReactor(tracker k8stesting.ObjectTracker) k8stesting.ReactionFunc {
	return func(action k8stesting.Action) (bool, runtime.Object, error) {
		pa, ok := action.(k8stesting.PatchAction)
		if !ok || pa.GetPatchType() != types.ApplyPatchType {
			return false, nil, nil // let the default reactor handle it
		}
		obj := &unstructured.Unstructured{}
		if err := json.Unmarshal(pa.GetPatch(), &obj.Object); err != nil {
			return true, nil, err
		}
		obj.SetName(pa.GetName())
		// Try create; if already exists, update.
		err := tracker.Create(pa.GetResource(), obj, pa.GetNamespace())
		if apierrors.IsAlreadyExists(err) {
			err = tracker.Update(pa.GetResource(), obj, pa.GetNamespace())
		}
		if err != nil {
			return true, nil, err
		}
		got, err := tracker.Get(pa.GetResource(), pa.GetNamespace(), pa.GetName(), metav1.GetOptions{})
		return true, got, err
	}
}

func TestReconcilerApplyCreatesNRT(t *testing.T) {
	client := newFakeClient()
	// The fake tracker's Apply requires the object to already exist (not an
	// upsert). Prepend a reactor that performs a real create-or-update so that
	// Reconciler.Apply (which uses the production server-side-apply path) can
	// be tested end-to-end against the fake.
	client.PrependReactor("patch", "noderesourcetopologies", applyUpsertReactor(client.Tracker()))
	r := NewReconciler(client)

	require.NoError(t, r.Apply(context.Background(), sampleNRT("node-a")))

	got, err := client.Resource(nrtGVR).Get(context.Background(), "node-a", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "node-a", got.GetName())
}
