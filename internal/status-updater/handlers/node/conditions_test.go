package node

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kfake "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func nodeWithConditions(name string, conditions ...v1.NodeCondition) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status:     v1.NodeStatus{Conditions: conditions},
	}
}

func conditionsOf(t *testing.T, handler *NodeHandler, name string) []v1.NodeCondition {
	t.Helper()
	node, err := handler.kubeClient.CoreV1().Nodes().Get(context.TODO(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get node %s: %v", name, err)
	}
	return node.Status.Conditions
}

func findCondition(conditions []v1.NodeCondition, conditionType v1.NodeConditionType) *v1.NodeCondition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

func TestSetGpuFractioningReadyCondition(t *testing.T) {
	kubeReady := v1.NodeCondition{Type: v1.NodeReady, Status: v1.ConditionTrue}

	for _, tc := range []struct {
		name       string
		existing   []v1.NodeCondition
		wantStatus v1.ConditionStatus
		wantReason string
	}{
		{name: "condition absent", existing: []v1.NodeCondition{kubeReady}, wantStatus: v1.ConditionTrue, wantReason: simulatedGpuFractioningReason},
		{
			name: "simulated condition present but false",
			existing: []v1.NodeCondition{kubeReady, {
				Type:   gpuFractioningReadyConditionType,
				Status: v1.ConditionFalse,
				Reason: simulatedGpuFractioningReason,
			}},
			wantStatus: v1.ConditionTrue,
			wantReason: simulatedGpuFractioningReason,
		},
		{
			name: "external condition present but false",
			existing: []v1.NodeCondition{kubeReady, {
				Type:   gpuFractioningReadyConditionType,
				Status: v1.ConditionFalse,
				Reason: "GPUDriverVersionUnsupported",
			}},
			wantStatus: v1.ConditionFalse,
			wantReason: "GPUDriverVersionUnsupported",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := nodeWithConditions("n1", tc.existing...)
			handler := &NodeHandler{kubeClient: kfake.NewSimpleClientset(node)}

			if err := handler.setGpuFractioningReadyCondition(node); err != nil {
				t.Fatalf("setGpuFractioningReadyCondition: %v", err)
			}

			conditions := conditionsOf(t, handler, "n1")
			got := findCondition(conditions, gpuFractioningReadyConditionType)
			if got == nil {
				t.Fatalf("condition %s was not written", gpuFractioningReadyConditionType)
			}
			if got.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tc.wantStatus)
			}
			if got.Reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", got.Reason, tc.wantReason)
			}
			if findCondition(conditions, v1.NodeReady) == nil {
				t.Error("kubelet-owned Ready condition was dropped")
			}
		})
	}
}

func TestRemoveGpuFractioningReadyCondition(t *testing.T) {
	node := nodeWithConditions("n1",
		v1.NodeCondition{Type: v1.NodeReady, Status: v1.ConditionTrue},
		v1.NodeCondition{Type: gpuFractioningReadyConditionType, Status: v1.ConditionTrue, Reason: simulatedGpuFractioningReason},
	)
	handler := &NodeHandler{kubeClient: kfake.NewSimpleClientset(node)}

	if err := handler.removeGpuFractioningReadyCondition(node); err != nil {
		t.Fatalf("removeGpuFractioningReadyCondition: %v", err)
	}

	conditions := conditionsOf(t, handler, "n1")
	if findCondition(conditions, gpuFractioningReadyConditionType) != nil {
		t.Errorf("condition %s was not removed", gpuFractioningReadyConditionType)
	}
	if findCondition(conditions, v1.NodeReady) == nil {
		t.Error("kubelet-owned Ready condition was dropped")
	}
}

func TestRemoveGpuFractioningReadyConditionUsesLiveOwnership(t *testing.T) {
	for _, tc := range []struct {
		name       string
		stale      v1.NodeCondition
		live       v1.NodeCondition
		wantExists bool
	}{
		{
			name:  "condition gained after event",
			stale: v1.NodeCondition{Type: v1.NodeReady, Status: v1.ConditionTrue},
			live:  v1.NodeCondition{Type: gpuFractioningReadyConditionType, Status: v1.ConditionTrue, Reason: simulatedGpuFractioningReason},
		},
		{
			name:       "another controller replaced condition",
			stale:      v1.NodeCondition{Type: gpuFractioningReadyConditionType, Status: v1.ConditionTrue, Reason: simulatedGpuFractioningReason},
			live:       v1.NodeCondition{Type: gpuFractioningReadyConditionType, Status: v1.ConditionFalse, Reason: "GPUDriverVersionUnsupported"},
			wantExists: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stale := nodeWithConditions("n1", tc.stale)
			live := nodeWithConditions("n1", tc.live)
			handler := &NodeHandler{kubeClient: kfake.NewSimpleClientset(live)}

			if err := handler.removeGpuFractioningReadyCondition(stale); err != nil {
				t.Fatalf("removeGpuFractioningReadyCondition: %v", err)
			}
			got := findCondition(conditionsOf(t, handler, "n1"), gpuFractioningReadyConditionType)
			if (got != nil) != tc.wantExists {
				t.Fatalf("condition exists = %v, want %v", got != nil, tc.wantExists)
			}
			if tc.wantExists && got.Reason != tc.live.Reason {
				t.Errorf("condition reason = %q, want %q", got.Reason, tc.live.Reason)
			}
		})
	}
}

func TestRemoveGpuFractioningReadyConditionPreservesConcurrentReplacement(t *testing.T) {
	owned := v1.NodeCondition{Type: gpuFractioningReadyConditionType, Status: v1.ConditionTrue, Reason: simulatedGpuFractioningReason}
	node := nodeWithConditions("n1", owned)
	client := kfake.NewSimpleClientset(node)
	client.PrependReactor("patch", "nodes", func(action ktesting.Action) (bool, runtime.Object, error) {
		resource := v1.SchemeGroupVersion.WithResource("nodes")
		current, err := client.Tracker().Get(resource, "", "n1")
		if err != nil {
			return true, nil, err
		}
		replacement := current.(*v1.Node).DeepCopy()
		replacement.Status.Conditions[0].Reason = "GPUDriverVersionUnsupported"
		if err := client.Tracker().Update(resource, replacement, ""); err != nil {
			return true, nil, err
		}
		return false, nil, nil
	})
	handler := &NodeHandler{kubeClient: client}

	if err := handler.removeGpuFractioningReadyCondition(node); err != nil {
		t.Fatalf("removeGpuFractioningReadyCondition: %v", err)
	}
	got := findCondition(conditionsOf(t, handler, "n1"), gpuFractioningReadyConditionType)
	if got == nil || got.Reason != "GPUDriverVersionUnsupported" {
		t.Fatalf("concurrent condition was removed or changed: %+v", got)
	}
}

func TestHandleDeleteRemovesConditionAfterTopologyFailure(t *testing.T) {
	node := nodeWithConditions("n1", v1.NodeCondition{
		Type: gpuFractioningReadyConditionType, Status: v1.ConditionTrue, Reason: simulatedGpuFractioningReason,
	})
	client := kfake.NewSimpleClientset(node)
	client.PrependReactor("delete", "configmaps", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("topology deletion failed")
	})
	handler := &NodeHandler{kubeClient: client, disableLabeling: true}

	err := handler.HandleDelete(node)
	if err == nil || !strings.Contains(err.Error(), "topology deletion failed") {
		t.Fatalf("HandleDelete error = %v, want topology deletion failure", err)
	}
	if got := findCondition(conditionsOf(t, handler, "n1"), gpuFractioningReadyConditionType); got != nil {
		t.Errorf("condition remained after topology deletion failed: %+v", *got)
	}
}

func TestHandleDeleteRemovesConditionAfterUnlabelFailure(t *testing.T) {
	node := nodeWithConditions("n1", v1.NodeCondition{
		Type: gpuFractioningReadyConditionType, Status: v1.ConditionTrue, Reason: simulatedGpuFractioningReason,
	})
	client := kfake.NewSimpleClientset(node)
	client.PrependReactor("patch", "nodes", func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() == "" {
			return true, nil, errors.New("unlabel failed")
		}
		return false, nil, nil
	})
	handler := &NodeHandler{kubeClient: client}

	err := handler.HandleDelete(node)
	if err == nil || !strings.Contains(err.Error(), "unlabel failed") {
		t.Fatalf("HandleDelete error = %v, want unlabel failure", err)
	}
	if got := findCondition(conditionsOf(t, handler, "n1"), gpuFractioningReadyConditionType); got != nil {
		t.Errorf("condition remained after unlabel failed: %+v", *got)
	}
}

func TestStaleUpdateDoesNotRestoreConditionAfterPoolExit(t *testing.T) {
	stale := nodeWithConditions("n1", v1.NodeCondition{
		Type: gpuFractioningReadyConditionType, Status: v1.ConditionTrue, Reason: simulatedGpuFractioningReason,
	})
	stale.Labels = map[string]string{"pool": "test"}
	stale.Status.Allocatable = v1.ResourceList{v1.ResourceName("nvidia.com/gpu"): resource.MustParse("8")}
	live := stale.DeepCopy()
	delete(live.Labels, "pool")
	client := kfake.NewSimpleClientset(live)
	handler := &NodeHandler{
		kubeClient: client,
		clusterConfig: &topology.ClusterConfig{
			NodePoolLabelKey: "pool",
			NodePools: map[string]topology.NodePoolConfig{
				"test": {Gpu: topology.GpuConfig{Backend: "fake"}},
			},
		},
	}

	if err := handler.HandleUpdate(stale); err != nil {
		t.Fatalf("HandleUpdate: %v", err)
	}
	if got := findCondition(conditionsOf(t, handler, "n1"), gpuFractioningReadyConditionType); got != nil {
		t.Errorf("condition remained after pool exit: %+v", *got)
	}
}

func TestReconcileGpuFractioningReadyCondition(t *testing.T) {
	for _, tc := range []struct {
		name       string
		backend    string
		gpuCount   string
		existing   bool
		external   bool
		wantExists bool
	}{
		{name: "fake GPU", backend: "fake", gpuCount: "8", wantExists: true},
		{name: "mock GPU", backend: "mock", gpuCount: "8"},
		{name: "remove simulated condition from mock GPU", backend: "mock", gpuCount: "8", existing: true},
		{name: "fake GPU before allocation", backend: "fake", gpuCount: "0"},
		{name: "remove simulated condition when GPU disappears", backend: "fake", gpuCount: "0", existing: true},
		{name: "preserve external condition on mock GPU", backend: "mock", gpuCount: "8", external: true, wantExists: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := nodeWithConditions("n1", v1.NodeCondition{Type: v1.NodeReady, Status: v1.ConditionTrue})
			node.Labels = map[string]string{"pool": "test"}
			node.Status.Allocatable = v1.ResourceList{v1.ResourceName("nvidia.com/gpu"): resource.MustParse(tc.gpuCount)}
			if tc.existing {
				node.Status.Conditions = append(node.Status.Conditions, v1.NodeCondition{
					Type: gpuFractioningReadyConditionType, Status: v1.ConditionTrue, Reason: simulatedGpuFractioningReason,
				})
			}
			if tc.external {
				node.Status.Conditions = append(node.Status.Conditions, v1.NodeCondition{
					Type: gpuFractioningReadyConditionType, Status: v1.ConditionFalse, Reason: "GPUDriverVersionUnsupported",
				})
			}
			handler := &NodeHandler{
				kubeClient: kfake.NewSimpleClientset(node),
				clusterConfig: &topology.ClusterConfig{
					NodePoolLabelKey: "pool",
					NodePools:        map[string]topology.NodePoolConfig{"test": {Gpu: topology.GpuConfig{Backend: tc.backend}}},
				},
			}
			if err := handler.HandleUpdate(node); err != nil {
				t.Fatalf("HandleUpdate: %v", err)
			}
			conditions := conditionsOf(t, handler, "n1")
			got := findCondition(conditions, gpuFractioningReadyConditionType)
			if (got != nil) != tc.wantExists {
				t.Errorf("condition exists = %v, want %v", got != nil, tc.wantExists)
			}
			if tc.external && got != nil && (got.Status != v1.ConditionFalse || got.Reason != "GPUDriverVersionUnsupported") {
				t.Errorf("external condition changed: %+v", *got)
			}
			if findCondition(conditions, v1.NodeReady) == nil {
				t.Error("kubelet-owned Ready condition was dropped")
			}
		})
	}
}
