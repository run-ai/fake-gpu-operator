package node

import (
	"errors"
	"fmt"
	"log"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
)

type Interface interface {
	HandleAdd(node *v1.Node) error
	HandleUpdate(node *v1.Node) error
	HandleDelete(node *v1.Node) error
}

type NodeHandler struct {
	kubeClient kubernetes.Interface

	clusterConfig   *topology.ClusterConfig
	disableLabeling bool
}

var _ Interface = &NodeHandler{}

func NewNodeHandler(kubeClient kubernetes.Interface, clusterConfig *topology.ClusterConfig, disableLabeling bool) *NodeHandler {
	return &NodeHandler{
		kubeClient:      kubeClient,
		clusterConfig:   clusterConfig,
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
	} else {
		err = p.labelNode(node)
		if err != nil {
			return fmt.Errorf("failed to label node: %w", err)
		}
	}

	err = p.reconcileGpuFractioningReadyCondition(node)
	if err != nil {
		return fmt.Errorf("failed to reconcile GPU fractioning readiness: %w", err)
	}

	return nil
}

func (p *NodeHandler) HandleDelete(node *v1.Node) error {
	log.Printf("Handling node deletion: %s\n", node.Name)

	var cleanupErrors []error
	if err := topology.DeleteNodeTopologyCM(p.kubeClient, node.Name); err != nil && !apierrors.IsNotFound(err) {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to delete node topology: %w", err))
	}

	if p.disableLabeling {
		log.Printf("Skipping node unlabeling for %s (disabled via config)\n", node.Name)
	} else {
		if err := p.unlabelNode(node); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to unlabel node: %w", err))
		}
	}

	if err := p.removeGpuFractioningReadyCondition(node); err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to remove GPU fractioning readiness: %w", err))
	}

	return errors.Join(cleanupErrors...)
}

// Re-assert on update: KWOK and the kubelet rewrite node status on their own schedule and drop
// conditions they do not know about.
func (p *NodeHandler) HandleUpdate(node *v1.Node) error {
	return p.reconcileGpuFractioningReadyCondition(node)
}
