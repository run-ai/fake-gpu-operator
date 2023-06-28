package node

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

type Interface interface {
	HandleAdd(node *v1.Node) error
	HandleDelete(node *v1.Node) error
}

type NodeHandler struct {
	kubeClient kubernetes.Interface
}

var _ Interface = &NodeHandler{}

func NewNodeHandler(kubeClient kubernetes.Interface) *NodeHandler {
	return &NodeHandler{
		kubeClient: kubeClient,
	}
}

func (p *NodeHandler) HandleAdd(node *v1.Node) error {
	log.Printf("Handling node addition: %s\n", node.Name)

	nodeTopology, _ := topology.GetNodeTopologyFromCM(p.kubeClient, node.Name)
	if nodeTopology != nil {
		return nil
	}

	clusterTopology, err := topology.GetFromKube(p.kubeClient)
	if err != nil {
		return fmt.Errorf("failed to get cluster topology: %w", err)
	}

	nodeAutofillSettings := clusterTopology.Config.NodeAutofill

	nodeTopology = &topology.Node{
		GpuMemory:   nodeAutofillSettings.GpuMemory,
		GpuProduct:  nodeAutofillSettings.GpuProduct,
		Gpus:        generateGpuDetails(nodeAutofillSettings.GpuCount, node.Name),
		MigStrategy: clusterTopology.MigStrategy,
	}

	clusterTopology.Nodes[node.Name] = *nodeTopology
	err = topology.CreateNodeTopologyCM(p.kubeClient, nodeTopology, node.Name)
	if err != nil {
		return fmt.Errorf("failed to create node topology: %w", err)
	}

	err = topology.UpdateToKube(p.kubeClient, clusterTopology)
	if err != nil {
		return fmt.Errorf("failed to update cluster topology: %w", err)
	}

	return nil
}

func (p *NodeHandler) HandleDelete(node *v1.Node) error {
	log.Printf("Handling node deletion: %s\n", node.Name)

	clusterTopology, err := topology.GetFromKube(p.kubeClient)
	if err != nil {
		return fmt.Errorf("failed to get cluster topology: %w", err)
	}

	nodeTopology, _ := topology.GetNodeTopologyFromCM(p.kubeClient, node.Name)
	if nodeTopology != nil {
		return nil
	}

	delete(clusterTopology.Nodes, node.Name)

	err = topology.UpdateToKube(p.kubeClient, clusterTopology)
	if err != nil {
		return fmt.Errorf("failed to update cluster topology: %w", err)
	}

	err = topology.DeleteNodeTopologyCM(p.kubeClient, node.Name)
	if err != nil {
		return fmt.Errorf("failed to delete node topology: %w", err)
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
