package node

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const (
	// AnnotationGpuFakeDevices is the annotation key for GPU fake devices on nodes.
	// This is the same annotation used by the DRA plugin to discover available GPUs.
	AnnotationGpuFakeDevices = "nvidia.com/gpu.fake.devices"
)

// AnnotateNodeWithTopology annotates the node with the GPU topology as JSON.
// This allows the DRA plugin to read the topology directly from the node annotation.
func AnnotateNodeWithTopology(kubeclient kubernetes.Interface, nodeTopology *topology.NodeTopology, nodeName string) error {
	topologyJSON, err := json.Marshal(nodeTopology)
	if err != nil {
		return fmt.Errorf("failed to marshal node topology: %w", err)
	}

	patch := map[string]interface{}{
		"metadata": map[string]interface{}{
			"annotations": map[string]interface{}{
				AnnotationGpuFakeDevices: string(topologyJSON),
			},
		},
	}
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	_, err = kubeclient.CoreV1().Nodes().Patch(context.TODO(), nodeName, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("failed to patch node %s with topology annotation: %w", nodeName, err)
	}

	return nil
}

// annotateNodeWithTopology is a method on NodeHandler that annotates the node with topology.
func (p *NodeHandler) annotateNodeWithTopology(node *v1.Node, nodeTopology *topology.NodeTopology) error {
	return AnnotateNodeWithTopology(p.kubeClient, nodeTopology, node.Name)
}
