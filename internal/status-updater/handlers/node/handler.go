package node

import (
	"fmt"
	"log"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
)

type Interface interface {
	HandleAdd(node *v1.Node) error
	HandleDelete(node *v1.Node) error
	HandleUpdate(node *v1.Node) error
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

	baseTopology, err := topology.GetBaseTopologyFromCM(p.kubeClient)
	if err != nil {
		return fmt.Errorf("failed to get base topology: %w", err)
	}

	err = p.createNodeTopologyCM(node, baseTopology)
	if err != nil {
		return fmt.Errorf("failed to create node topology ConfigMap: %w", err)
	}

	if baseTopology.Config.FakeNodeHandling {
		err = p.applyFakeDevicePlugin(baseTopology.Config.NodeAutofill.GpuCount, node)
		if err != nil {
			return fmt.Errorf("failed to apply fake node deployments: %w", err)
		}
	} else {
		err = p.applyFakeNodeDeployments(node)
		if err != nil {
			return fmt.Errorf("failed to apply fake node deployments: %w", err)
		}
	}

	return nil
}

func (p *NodeHandler) HandleDelete(node *v1.Node) error {
	log.Printf("Handling node deletion: %s\n", node.Name)

	err := topology.DeleteNodeTopologyCM(p.kubeClient, node.Name)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete node topology: %w", err)
	}

	err = p.deleteFakeNodeDeployments(node)
	if err != nil {
		return fmt.Errorf("failed to delete fake node deployments: %w", err)
	}

	return nil
}

func (p *NodeHandler) HandleUpdate(node *v1.Node) error {
	baseTopology, err := topology.GetBaseTopologyFromCM(p.kubeClient)
	if err != nil {
		return fmt.Errorf("failed to get base topology: %w", err)
	}

	if !baseTopology.Config.FakeNodeHandling {
		return nil
	}

	gpuCount := baseTopology.Config.NodeAutofill.GpuCount
	err = p.applyFakeDevicePlugin(gpuCount, node)
	if err != nil {
		return fmt.Errorf("failed to apply fake node deployments: %w", err)
	}
	return nil
}
