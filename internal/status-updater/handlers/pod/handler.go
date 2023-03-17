package pod

import (
	"fmt"
	"log"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
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

	clusterTopology, err := topology.GetFromKube(p.kubeClient)
	if err != nil {
		return fmt.Errorf("error getting topology: %v", err)
	}

	_, ok := clusterTopology.Nodes[pod.Spec.NodeName]
	if !ok {
		return fmt.Errorf("node %s not found in topology", pod.Spec.NodeName)
	}

	err = p.handleDedicatedGpuPodAddition(pod, clusterTopology)
	if err != nil {
		return err
	}

	err = p.handleSharedGpuPodAddition(pod, clusterTopology)
	if err != nil {
		return err
	}

	return topology.UpdateToKube(p.kubeClient, clusterTopology)
}

func (p *PodHandler) HandleUpdate(pod *v1.Pod) error {
	log.Printf("Handling pod update: %s\n", pod.Name)

	clusterTopology, err := topology.GetFromKube(p.kubeClient)
	if err != nil {
		return fmt.Errorf("error getting topology: %v", err)
	}

	_, ok := clusterTopology.Nodes[pod.Spec.NodeName]
	if !ok {
		return fmt.Errorf("node %s not found in topology", pod.Spec.NodeName)
	}

	err = p.handleDedicatedGpuPodUpdate(pod, clusterTopology)
	if err != nil {
		return err
	}

	err = p.handleSharedGpuPodUpdate(pod, clusterTopology)
	if err != nil {
		return err
	}

	return topology.UpdateToKube(p.kubeClient, clusterTopology)
}

func (p *PodHandler) HandleDelete(pod *v1.Pod) error {
	log.Printf("Handling pod deletion: %s\n", pod.Name)

	clusterTopology, err := topology.GetFromKube(p.kubeClient)
	if err != nil {
		return fmt.Errorf("error getting topology: %v", err)
	}

	_, ok := clusterTopology.Nodes[pod.Spec.NodeName]
	if !ok {
		return fmt.Errorf("node %s not found in topology", pod.Spec.NodeName)
	}

	p.handleDedicatedGpuPodDeletion(pod, clusterTopology)

	err = p.handleSharedGpuPodDeletion(pod, clusterTopology)
	if err != nil {
		return err
	}

	return topology.UpdateToKube(p.kubeClient, clusterTopology)
}
