package node

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
)

func (p *NodeHandler) createNodeTopologyCM(node *v1.Node) error {
	nodeTopology, err := topology.GetNodeTopologyFromCM(p.kubeClient, node.Name)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get node topology for node %s: %w", node.Name, err)
	}
	if nodeTopology != nil {
		return nil
	}

	nodePoolName, ok := node.Labels[p.clusterConfig.NodePoolLabelKey]
	if !ok {
		return fmt.Errorf("node %s does not have a nodepool label", node.Name)
	}

	poolConfig, ok := p.clusterConfig.NodePools[nodePoolName]
	if !ok {
		return fmt.Errorf("nodepool %s not found in cluster topology", nodePoolName)
	}

	namespace := viper.GetString(constants.EnvTopologyCmNamespace)
	resolved, err := topology.ResolveNodePool(p.kubeClient, namespace, poolConfig)
	if err != nil {
		return fmt.Errorf("failed to resolve nodepool %s: %w", nodePoolName, err)
	}

	nodeTopology = &topology.NodeTopology{
		GpuMemory:     resolved.GpuMemory,
		GpuProduct:    resolved.GpuProduct,
		DriverVersion: resolved.DriverVersion,
		CudaVersion:   resolved.CudaVersion,
		Gpus:          generateGpuDetails(resolved.GpuCount, node.Name),
		MigStrategy:   p.clusterConfig.MigStrategy,
		OtherDevices:  resolved.OtherDevices,
	}

	err = topology.CreateNodeTopologyCM(p.kubeClient, nodeTopology, node)
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
