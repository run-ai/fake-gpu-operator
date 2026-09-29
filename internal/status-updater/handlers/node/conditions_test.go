package node

import (
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
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
		name     string
		existing []v1.NodeCondition
	}{
		{name: "condition absent", existing: []v1.NodeCondition{kubeReady}},
		{
			name: "condition present but false",
			existing: []v1.NodeCondition{kubeReady, {
				Type:   gpuFractioningReadyConditionType,
				Status: v1.ConditionFalse,
				Reason: "GPUDriverVersionUnsupported",
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := &NodeHandler{kubeClient: kfake.NewSimpleClientset(nodeWithConditions("n1", tc.existing...))}

			if err := handler.setGpuFractioningReadyCondition("n1"); err != nil {
				t.Fatalf("setGpuFractioningReadyCondition: %v", err)
			}

			conditions := conditionsOf(t, handler, "n1")
			got := findCondition(conditions, gpuFractioningReadyConditionType)
			if got == nil {
				t.Fatalf("condition %s was not written", gpuFractioningReadyConditionType)
			}
			if got.Status != v1.ConditionTrue {
				t.Errorf("status = %q, want %q", got.Status, v1.ConditionTrue)
			}
			if got.Reason != "AllDaemonsReady" {
				t.Errorf("reason = %q, want AllDaemonsReady", got.Reason)
			}
			if findCondition(conditions, v1.NodeReady) == nil {
				t.Error("kubelet-owned Ready condition was dropped")
			}
		})
	}
}

func TestRemoveGpuFractioningReadyCondition(t *testing.T) {
	handler := &NodeHandler{kubeClient: kfake.NewSimpleClientset(nodeWithConditions("n1",
		v1.NodeCondition{Type: v1.NodeReady, Status: v1.ConditionTrue},
		v1.NodeCondition{Type: gpuFractioningReadyConditionType, Status: v1.ConditionTrue},
	))}

	if err := handler.removeGpuFractioningReadyCondition("n1"); err != nil {
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
