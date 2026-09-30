package node

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const gpuFractioningReadyConditionType = "gpu-fractioning.nvidia.com/Ready"
const simulatedGpuFractioningReason = "FakeGpuSimulation"

func (p *NodeHandler) reconcileGpuFractioningReadyCondition(node *v1.Node) error {
	current, err := p.kubeClient.CoreV1().Nodes().Get(context.TODO(), node.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get node %s: %w", node.Name, err)
	}

	poolName := current.Labels[p.clusterConfig.NodePoolLabelKey]
	pool, found := p.clusterConfig.NodePools[poolName]
	gpu, hasGpu := current.Status.Allocatable[v1.ResourceName("nvidia.com/gpu")]
	if !found || pool.Gpu.Backend != constants.BackendFake || !hasGpu || gpu.Sign() <= 0 {
		return p.removeGpuFractioningReadyCondition(current)
	}
	return p.setGpuFractioningReadyCondition(current)
}

func (p *NodeHandler) setGpuFractioningReadyCondition(node *v1.Node) error {
	for _, existing := range node.Status.Conditions {
		if existing.Type == gpuFractioningReadyConditionType {
			if existing.Reason != simulatedGpuFractioningReason || existing.Status == v1.ConditionTrue {
				return nil
			}
		}
	}

	now := metav1.Now()
	err := p.patchNodeConditions(node.Name, map[string]interface{}{
		"type":               gpuFractioningReadyConditionType,
		"status":             string(v1.ConditionTrue),
		"reason":             simulatedGpuFractioningReason,
		"message":            "fake GPU fractioning readiness is simulated",
		"lastHeartbeatTime":  now,
		"lastTransitionTime": now,
	})
	if err != nil {
		return fmt.Errorf("failed to set %s on node %s: %w", gpuFractioningReadyConditionType, node.Name, err)
	}

	return nil
}

func (p *NodeHandler) removeGpuFractioningReadyCondition(node *v1.Node) error {
	var patchError error
	for attempt := 0; attempt < 3; attempt++ {
		current, err := p.kubeClient.CoreV1().Nodes().Get(context.TODO(), node.Name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to get node %s: %w", node.Name, err)
		}

		found := false
		for i, existing := range current.Status.Conditions {
			if existing.Type != gpuFractioningReadyConditionType {
				continue
			}
			if existing.Reason != simulatedGpuFractioningReason {
				return nil
			}
			found = true

			path := fmt.Sprintf("/status/conditions/%d", i)
			patch, err := json.Marshal([]map[string]interface{}{
				{"op": "test", "path": path + "/type", "value": gpuFractioningReadyConditionType},
				{"op": "test", "path": path + "/reason", "value": simulatedGpuFractioningReason},
				{"op": "remove", "path": path},
			})
			if err != nil {
				return fmt.Errorf("failed to marshal %s removal patch: %w", gpuFractioningReadyConditionType, err)
			}

			_, err = p.kubeClient.CoreV1().Nodes().Patch(context.TODO(), node.Name, types.JSONPatchType, patch, metav1.PatchOptions{}, "status")
			if err == nil || errors.IsNotFound(err) {
				return nil
			}
			patchError = err
			break
		}
		if !found {
			return nil
		}
	}

	return fmt.Errorf("failed to remove %s from node %s: %w", gpuFractioningReadyConditionType, node.Name, patchError)
}

// Sending only our own entry lets the API server merge it by type, so the kubelet's conditions
// are never ours to hand back.
func (p *NodeHandler) patchNodeConditions(nodeName string, conditions ...map[string]interface{}) error {
	patch, err := json.Marshal(map[string]interface{}{
		"status": map[string]interface{}{"conditions": conditions},
	})
	if err != nil {
		return fmt.Errorf("failed to marshal conditions patch: %w", err)
	}

	_, err = p.kubeClient.CoreV1().Nodes().PatchStatus(context.TODO(), nodeName, patch)
	return err
}
