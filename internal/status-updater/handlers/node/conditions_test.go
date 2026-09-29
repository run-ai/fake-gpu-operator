package node

import (
	"context"
	"testing"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kfake "k8s.io/client-go/kubernetes/fake"
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

func TestReconcileGpuFractioningReadyCondition(t *testing.T) {
	for _, tc := range []struct {
		name       string
		backend    string
		gpuCount   string
		enabled    bool
		existing   bool
		external   bool
		wantExists bool
	}{
		{name: "fake GPU with opt-in", backend: "fake", gpuCount: "8", enabled: true, wantExists: true},
		{name: "mock GPU with opt-in", backend: "mock", gpuCount: "8", enabled: true},
		{name: "remove simulated condition from mock GPU", backend: "mock", gpuCount: "8", enabled: true, existing: true},
		{name: "fake GPU without opt-in", backend: "fake", gpuCount: "8"},
		{name: "remove simulated condition when opt-in is disabled", backend: "fake", gpuCount: "8", existing: true},
		{name: "fake GPU before allocation", backend: "fake", gpuCount: "0", enabled: true},
		{name: "remove simulated condition when GPU disappears", backend: "fake", gpuCount: "0", enabled: true, existing: true},
		{name: "preserve external condition on mock GPU", backend: "mock", gpuCount: "8", enabled: true, external: true, wantExists: true},
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
				simulateGpuFractioningReady: tc.enabled,
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
