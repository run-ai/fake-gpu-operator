/*
Subscribes to pod updates and handles them.
When a pod is added, we'll mark a random GPU on the pod's node as fully utilized
When a pod is removed, we'll unmark the GPU.
*/
package handle

import (
	"fmt"
	"log"
	"os"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/inform"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

type Interface interface {
	Run(stopCh <-chan struct{})
}

type PodEventHandler struct {
	podEvents  <-chan *inform.PodEvent
	kubeclient kubernetes.Interface
}

var _ Interface = &PodEventHandler{}

func NewPodEventHandler(kubeclient kubernetes.Interface, informer inform.Interface) *PodEventHandler {
	podEvents := make(chan *inform.PodEvent)
	informer.Subscribe(podEvents)

	p := &PodEventHandler{
		podEvents:  podEvents,
		kubeclient: kubeclient,
	}

	err := p.resetTopologyStatus()
	if err != nil {
		log.Printf("Error resetting topology status: %v\n", err)
		log.Printf("Please reset the topology status manually\n")
		log.Printf("Exiting...\n")
		os.Exit(1)
	}

	return p
}

func (p *PodEventHandler) Run(stopCh <-chan struct{}) {
	p.processPodEvents(stopCh)
}

func (p *PodEventHandler) processPodEvents(stopCh <-chan struct{}) {
	for {
		select {
		case podEvent := <-p.podEvents:
			log.Printf("Processing pod: %s\n", podEvent.Pod.Name)

			topologyCm, clusterTopology, err := p.getTopology()
			if err != nil {
				log.Printf("Error getting topology: %v\n", err)
				return
			}

			_, ok := clusterTopology.Nodes[podEvent.Pod.Spec.NodeName]
			if !ok {
				log.Printf("Node %s not found in topology\n", podEvent.Pod.Spec.NodeName)
				continue
			}

			if podEvent.EventType == inform.ADD {
				err = p.handleAdd(podEvent, clusterTopology, topologyCm)
				if err != nil {
					log.Printf("Error handling pod add: %v\n", err)
					return
				}
			} else if podEvent.EventType == inform.DELETE {
				err := p.handleDelete(podEvent, clusterTopology, topologyCm)
				if err != nil {
					log.Printf("Error handling pod delete: %v\n", err)
					return
				}
			}

		case <-stopCh:
			log.Println("Stopping pod event processor")
			return
		}
	}
}

func (p *PodEventHandler) handleAdd(podEvent *inform.PodEvent, clusterTopology *topology.ClusterTopology, topologyCm *v1.ConfigMap) error {
	requestedGpus := podEvent.Pod.Spec.Containers[0].Resources.Limits.Name("nvidia.com/gpu", "")
	if requestedGpus == nil {
		return fmt.Errorf("no GPUs requested in pod %s", podEvent.Pod.Name)
	}

	requestedGpusCount := requestedGpus.Value()
	log.Printf("Requested GPUs: %d\n", requestedGpusCount)
	for idx, gpu := range clusterTopology.Nodes[podEvent.Pod.Spec.NodeName].Gpus {
		if gpu.Metrics.Status.Utilization == 0 {
			log.Printf("GPU %s is free, allocating...\n", gpu.ID)
			gpu.Metrics.Status.Utilization = 100
			gpu.Metrics.Status.FbUsed = clusterTopology.Nodes[podEvent.Pod.Spec.NodeName].GpuMemory
			gpu.Metrics.Metadata.Namespace = podEvent.Pod.Namespace
			gpu.Metrics.Metadata.Pod = podEvent.Pod.Name
			gpu.Metrics.Metadata.Container = podEvent.Pod.Spec.Containers[0].Name

			clusterTopology.Nodes[podEvent.Pod.Spec.NodeName].Gpus[idx] = gpu

			requestedGpusCount--
		}

		if requestedGpusCount <= 0 {
			break
		}
	}

	return p.updateTopology(clusterTopology, topologyCm)
}

func (p *PodEventHandler) handleDelete(podEvent *inform.PodEvent, clusterTopology *topology.ClusterTopology, topologyCm *v1.ConfigMap) error {
	for idx, gpu := range clusterTopology.Nodes[podEvent.Pod.Spec.NodeName].Gpus {
		isGpuOccupiedByPod := gpu.Metrics.Metadata.Namespace == podEvent.Pod.Namespace &&
			gpu.Metrics.Metadata.Pod == podEvent.Pod.Name &&
			gpu.Metrics.Metadata.Container == podEvent.Pod.Spec.Containers[0].Name
		if isGpuOccupiedByPod {
			clusterTopology.Nodes[podEvent.Pod.Spec.NodeName].Gpus[idx].Metrics = topology.GpuMetrics{}
		}
	}

	return p.updateTopology(clusterTopology, topologyCm)
}
