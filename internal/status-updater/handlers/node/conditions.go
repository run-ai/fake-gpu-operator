package node

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
)

// KAI refuses to place a fractional workload on a node without this condition. On a real cluster
// the GPU fractioning operator writes it after verifying the driver and its daemons; nothing here
// is real enough to produce one, so simulate it alongside the GPUs themselves.
const gpuFractioningReadyConditionType = "gpu-fractioning.nvidia.com/Ready"

func (p *NodeHandler) setGpuFractioningReadyCondition(nodeName string) error {
	condition := v1.NodeCondition{
		Type:    gpuFractioningReadyConditionType,
		Status:  v1.ConditionTrue,
		Reason:  "AllDaemonsReady",
		Message: "all gpu-fractioning daemons are running",
	}

	err := p.updateNodeCondition(nodeName, func(conditions []v1.NodeCondition) ([]v1.NodeCondition, bool) {
		for i, existing := range conditions {
			if existing.Type != condition.Type {
				continue
			}
			if existing.Status == condition.Status {
				return conditions, false
			}
			condition.LastTransitionTime = metav1.Now()
			conditions[i] = condition
			return conditions, true
		}

		condition.LastTransitionTime = metav1.Now()
		return append(conditions, condition), true
	})
	if err != nil {
		return fmt.Errorf("failed to set %s on node %s: %w", gpuFractioningReadyConditionType, nodeName, err)
	}

	return nil
}

func (p *NodeHandler) removeGpuFractioningReadyCondition(nodeName string) error {
	err := p.updateNodeCondition(nodeName, func(conditions []v1.NodeCondition) ([]v1.NodeCondition, bool) {
		for i, existing := range conditions {
			if existing.Type == gpuFractioningReadyConditionType {
				return append(conditions[:i], conditions[i+1:]...), true
			}
		}
		return conditions, false
	})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to remove %s from node %s: %w", gpuFractioningReadyConditionType, nodeName, err)
	}

	return nil
}

// updateNodeCondition re-reads the node on conflict because the kubelet writes the same condition
// list on its own schedule, and a stale read would drop whatever it set in between.
func (p *NodeHandler) updateNodeCondition(
	nodeName string, mutate func([]v1.NodeCondition) ([]v1.NodeCondition, bool),
) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := p.kubeClient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}

		conditions, changed := mutate(node.Status.Conditions)
		if !changed {
			return nil
		}
		node.Status.Conditions = conditions

		_, err = p.kubeClient.CoreV1().Nodes().UpdateStatus(context.TODO(), node, metav1.UpdateOptions{})
		return err
	})
}
