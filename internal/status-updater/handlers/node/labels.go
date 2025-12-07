package node

import (
	"context"
	"encoding/json"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	dcgmExporterLabelKey = "nvidia.com/gpu.deploy.dcgm-exporter"
	devicePluginLabelKey = "nvidia.com/gpu.deploy.device-plugin"
	draPluginGpuLabelKey = "nvidia.com/gpu.deploy.dra-plugin-gpu"
)

// labelNode labels the node with required labels for the fake-gpu-operator to function.
func (p *NodeHandler) labelNode(node *v1.Node) error {
	labels := map[string]interface{}{
		dcgmExporterLabelKey: "true",
	}
	if !isFakeNode(node) {
		labels[devicePluginLabelKey] = "true"
		labels[draPluginGpuLabelKey] = "true"
	}

	err := p.patchNodeLabels(node, labels)
	if err != nil {
		return fmt.Errorf("failed to label node %s: %w", node.Name, err)
	}

	return nil
}

// unlabelNode removes the labels from the node that were added by the fake-gpu-operator.
func (p *NodeHandler) unlabelNode(node *v1.Node) error {
	err := p.patchNodeLabels(node, map[string]interface{}{
		dcgmExporterLabelKey: nil,
		devicePluginLabelKey: nil,
		draPluginGpuLabelKey: nil,
	})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to unlabel node %s: %w", node.Name, err)
	}

	return nil
}

func (p *NodeHandler) patchNodeLabels(node *v1.Node, labels map[string]interface{}) error {
	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"labels": labels,
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	_, err = p.kubeClient.CoreV1().Nodes().Patch(context.TODO(), node.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("failed to patch node %s labels: %w", node.Name, err)
	}

	return nil
}
