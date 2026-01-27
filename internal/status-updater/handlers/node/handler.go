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
}

type NodeHandler struct {
	kubeClient kubernetes.Interface

	clusterTopology *topology.ClusterTopology
	disableLabeling bool
}

var _ Interface = &NodeHandler{}

func NewNodeHandler(kubeClient kubernetes.Interface, clusterTopology *topology.ClusterTopology, disableLabeling bool) *NodeHandler {
	return &NodeHandler{
		kubeClient:      kubeClient,
		clusterTopology: clusterTopology,
		disableLabeling: disableLabeling,
	}
}

func (p *NodeHandler) HandleAdd(node *v1.Node) error {
	log.Printf("Handling node addition: %s\n", node.Name)

	err := p.createNodeTopologyCM(node)
	if err != nil {
		return fmt.Errorf("failed to create node topology ConfigMap: %w", err)
	}

	if p.disableLabeling {
		log.Printf("Skipping node labeling for %s (disabled via config)\n", node.Name)
		return nil
	}

	err = p.labelNode(node)
	if err != nil {
		return fmt.Errorf("failed to label node: %w", err)
	}

	return nil
}

func (p *NodeHandler) HandleDelete(node *v1.Node) error {
	log.Printf("Handling node deletion: %s\n", node.Name)

	err := topology.DeleteNodeTopologyCM(p.kubeClient, node.Name)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete node topology: %w", err)
	}

	if p.disableLabeling {
		log.Printf("Skipping node unlabeling for %s (disabled via config)\n", node.Name)
		return nil
	}

	err = p.unlabelNode(node)
	if err != nil {
		return fmt.Errorf("failed to unlabel node: %w", err)
	}

	return nil
}
