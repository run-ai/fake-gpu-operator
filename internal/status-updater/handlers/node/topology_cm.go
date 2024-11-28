package node

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	v1 "k8s.io/api/core/v1"
)

func (p *NodeHandler) createNodeTopologyCM(node *v1.Node) error {
	nodeTopology, _ := topology.GetNodeTopologyFromCM(p.kubeClient, node.Name)
	if nodeTopology != nil {
		return nil
	}

	nodePoolName, ok := node.Labels[p.clusterTopology.NodePoolLabelKey]
	if !ok {
		return fmt.Errorf("node %s does not have a nodepool label", node.Name)
	}

	nodePoolTopology, ok := p.clusterTopology.NodePools[nodePoolName]
	if !ok {
		return fmt.Errorf("nodepool %s not found in cluster topology", nodePoolName)
	}

	nodeTopology = &topology.NodeTopology{
		GpuMemory:    nodePoolTopology.GpuMemory,
		GpuProduct:   nodePoolTopology.GpuProduct,
		Gpus:         generateGpuDetails(nodePoolTopology.GpuCount, node.Name),
		MigStrategy:  p.clusterTopology.MigStrategy,
		OtherDevices: nodePoolTopology.OtherDevices,
	}

	err := topology.CreateNodeTopologyCM(p.kubeClient, nodeTopology, node)
	if err != nil {
		return fmt.Errorf("failed to create node topology: %w", err)
	}

	return nil
}

func generateGpuDetails(gpuCount int, nodeName string) []topology.GpuDetails {
	gpus := make([]topology.GpuDetails, gpuCount)
	for idx := range gpus {
		gpus[idx] = topology.GpuDetails{
			ID: fmt.Sprintf("GPU-%s", uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("%s-%d", nodeName, idx)))),
		}
	}

	return gpus
}
