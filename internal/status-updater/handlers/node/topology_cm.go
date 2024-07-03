package node

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	v1 "k8s.io/api/core/v1"
)

func (p *NodeHandler) createNodeTopologyCM(
	node *v1.Node, baseTopology *topology.BaseTopology,
) error {
	nodeTopology, _ := topology.GetNodeTopologyFromCM(p.kubeClient, node.Name)
	if nodeTopology != nil {
		return nil
	}

	nodeAutofillSettings := baseTopology.Config.NodeAutofill

	nodeTopology = &topology.NodeTopology{
		GpuMemory:   nodeAutofillSettings.GpuMemory,
		GpuProduct:  nodeAutofillSettings.GpuProduct,
		Gpus:        generateGpuDetails(nodeAutofillSettings.GpuCount, node.Name),
		MigStrategy: nodeAutofillSettings.MigStrategy,
	}

	err := topology.CreateNodeTopologyCM(p.kubeClient, nodeTopology, node.Name)
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
