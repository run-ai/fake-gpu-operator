package node

import (
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

	baseTopology, err := topology.GetBaseTopologyFromCM(p.kubeClient)
	if err != nil {
		return fmt.Errorf("failed to get base topology: %w", err)
	}

	nodeAutofillSettings := baseTopology.Config.NodeAutofill

	nodeTopology = &topology.NodeTopology{
		GpuMemory:   nodeAutofillSettings.GpuMemory,
		GpuProduct:  nodeAutofillSettings.GpuProduct,
		Gpus:        generateGpuDetails(nodeAutofillSettings.GpuCount, node.Name),
		MigStrategy: nodeAutofillSettings.MigStrategy,
	}

	err = topology.CreateNodeTopologyCM(p.kubeClient, nodeTopology, node.Name)
	if err != nil {
		return fmt.Errorf("failed to create node topology: %w", err)
	}

	return nil
}

func (p *NodeHandler) HandleDelete(node *v1.Node) error {
	log.Printf("Handling node deletion: %s\n", node.Name)

	err := topology.DeleteNodeTopologyCM(p.kubeClient, node.Name)
	if err != nil && !errors.IsNotFound(err) {
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
