package pod

import (
	"fmt"
	"log"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	nodehandler "github.com/run-ai/fake-gpu-operator/internal/status-updater/handlers/node"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type Interface interface {
	HandleAdd(pod *v1.Pod) error
	HandleUpdate(pod *v1.Pod) error
	HandleDelete(pod *v1.Pod) error
}

type PodHandler struct {
	kubeClient    kubernetes.Interface
	dynamicClient dynamic.Interface
}

var _ Interface = &PodHandler{}

func NewPodHandler(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface) *PodHandler {
	return &PodHandler{
		kubeClient:    kubeClient,
		dynamicClient: dynamicClient,
	}
}

func (p *PodHandler) HandleAdd(pod *v1.Pod) error {
	log.Printf("Handling pod addition: %s\n", pod.Name)

	nodeTopology, err := topology.GetNodeTopologyFromCM(p.kubeClient, pod.Spec.NodeName)
	if err != nil {
		return fmt.Errorf("could not get node %s topology: %w", pod.Spec.NodeName, err)
	}

	err = p.handleDedicatedGpuPodAddition(pod, nodeTopology)
	if err != nil {
		return err
	}

	err = p.handleSharedGpuPodAddition(pod, nodeTopology)
	if err != nil {
		return err
	}

	err = p.handleDraGpuPodAddition(pod, nodeTopology)
	if err != nil {
		return err
	}

	return p.updateNodeTopology(nodeTopology, pod.Spec.NodeName)
}

func (p *PodHandler) HandleUpdate(pod *v1.Pod) error {
	log.Printf("Handling pod update: %s\n", pod.Name)

	nodeTopology, err := topology.GetNodeTopologyFromCM(p.kubeClient, pod.Spec.NodeName)
	if err != nil {
		return fmt.Errorf("could not get node %s topology: %w", pod.Spec.NodeName, err)
	}

	err = p.handleDedicatedGpuPodUpdate(pod, nodeTopology)
	if err != nil {
		return err
	}

	err = p.handleSharedGpuPodUpdate(pod, nodeTopology)
	if err != nil {
		return err
	}

	err = p.handleDraGpuPodUpdate(pod, nodeTopology)
	if err != nil {
		return err
	}

	return p.updateNodeTopology(nodeTopology, pod.Spec.NodeName)
}

func (p *PodHandler) HandleDelete(pod *v1.Pod) error {
	log.Printf("Handling pod deletion: %s\n", pod.Name)

	nodeTopology, err := topology.GetNodeTopologyFromCM(p.kubeClient, pod.Spec.NodeName)
	if err != nil {
		return fmt.Errorf("could not get node %s topology: %w", pod.Spec.NodeName, err)
	}

	p.handleDedicatedGpuPodDeletion(pod, nodeTopology)

	err = p.handleSharedGpuPodDeletion(pod, nodeTopology)
	if err != nil {
		return err
	}

	p.handleDraGpuPodDeletion(pod, nodeTopology)

	return p.updateNodeTopology(nodeTopology, pod.Spec.NodeName)
}

// updateNodeTopology updates both the ConfigMap and the node annotation with the topology.
// The ConfigMap is kept for backwards compatibility, while the annotation is used by the DRA plugin.
func (p *PodHandler) updateNodeTopology(nodeTopology *topology.NodeTopology, nodeName string) error {
	// Update the ConfigMap (for backwards compatibility)
	if err := topology.UpdateNodeTopologyCM(p.kubeClient, nodeTopology, nodeName); err != nil {
		return fmt.Errorf("failed to update node topology CM: %w", err)
	}

	// Also update the node annotation (for DRA plugin compatibility)
	if err := nodehandler.AnnotateNodeWithTopology(p.kubeClient, nodeTopology, nodeName); err != nil {
		return fmt.Errorf("failed to annotate node with topology: %w", err)
	}

	return nil
}
