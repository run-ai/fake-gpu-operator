package node

import (
	"context"
	"encoding/json"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Fractional GPU scheduling is gated on this condition, so simulate it like the GPUs themselves.
const gpuFractioningReadyConditionType = "gpu-fractioning.nvidia.com/Ready"

func (p *NodeHandler) setGpuFractioningReadyCondition(node *v1.Node) error {
	for _, existing := range node.Status.Conditions {
		if existing.Type == gpuFractioningReadyConditionType && existing.Status == v1.ConditionTrue {
			return nil
		}
	}

	now := metav1.Now()
	err := p.patchNodeConditions(node.Name, map[string]interface{}{
		"type":               gpuFractioningReadyConditionType,
		"status":             string(v1.ConditionTrue),
		"reason":             "AllDaemonsReady",
		"message":            "all gpu-fractioning daemons are running",
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
		if existing.Type == gpuFractioningReadyConditionType {
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
