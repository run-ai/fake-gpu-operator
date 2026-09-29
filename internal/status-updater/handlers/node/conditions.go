package node

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const gpuFractioningReadyConditionType = "gpu-fractioning.nvidia.com/Ready"
const simulatedGpuFractioningReason = "FakeGpuSimulation"

func (p *NodeHandler) reconcileGpuFractioningReadyCondition(node *v1.Node) error {
	poolName := node.Labels[p.clusterConfig.NodePoolLabelKey]
	pool, found := p.clusterConfig.NodePools[poolName]
	gpu, hasGpu := node.Status.Allocatable[v1.ResourceName("nvidia.com/gpu")]
	if !found || pool.Gpu.Backend != constants.BackendFake || !hasGpu || gpu.Sign() <= 0 {
		return p.removeGpuFractioningReadyCondition(node)
	}
	return p.setGpuFractioningReadyCondition(node)
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
	found := false
	for _, existing := range node.Status.Conditions {
		if existing.Type == gpuFractioningReadyConditionType && existing.Reason == simulatedGpuFractioningReason {
			found = true
			break
		}
	}
	if !found {
		return nil
	}

	err := p.patchNodeConditions(node.Name, map[string]interface{}{
		"type":   gpuFractioningReadyConditionType,
		"$patch": "delete",
	})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to remove %s from node %s: %w", gpuFractioningReadyConditionType, node.Name, err)
	}

	return nil
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
