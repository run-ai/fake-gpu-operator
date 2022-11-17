/*
Subscribes to pod updates and handles them.
When a pod is added, we'll mark a random GPU on the pod's node as fully utilized
When a pod is removed, we'll unmark the GPU.
*/
package handle

import (
	"log"
	"os"
	"sync"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/inform"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type Interface interface {
	Run(stopCh <-chan struct{}, wg *sync.WaitGroup)
}

type PodEventHandler struct {
	podEvents     <-chan *inform.PodEvent
	kubeclient    kubernetes.Interface
	dynamicClient dynamic.Interface
}

var _ Interface = &PodEventHandler{}

func NewPodEventHandler(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, informer inform.Interface) *PodEventHandler {
	podEvents := make(chan *inform.PodEvent)
	informer.Subscribe(podEvents)

	p := &PodEventHandler{
		podEvents:     podEvents,
		kubeclient:    kubeClient,
		dynamicClient: dynamicClient,
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

func (p *PodEventHandler) Run(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
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
				}
			} else if podEvent.EventType == inform.DELETE {
				err := p.handleDelete(podEvent, clusterTopology, topologyCm)
				if err != nil {
					log.Printf("Error handling pod delete: %v\n", err)
				}
			}

		case <-stopCh:
			log.Println("Stopping pod event processor")
			return
		}
	}
}

func (p *PodEventHandler) handleAdd(podEvent *inform.PodEvent, clusterTopology *topology.ClusterTopology, topologyCm *v1.ConfigMap) error {
	err := p.handleDedicatedGpuPodAddition(podEvent.Pod, clusterTopology)
	if err != nil {
		return err
	}

	err = p.handleSharedGpuPodAddition(podEvent.Pod, clusterTopology)
	if err != nil {
		return err
	}

	return p.updateTopology(clusterTopology, topologyCm)
}

func (p *PodEventHandler) handleDelete(podEvent *inform.PodEvent, clusterTopology *topology.ClusterTopology, topologyCm *v1.ConfigMap) error {
	p.handleDedicatedGpuPodDeletion(podEvent.Pod, clusterTopology)

	err := p.handleSharedGpuPodDeletion(podEvent.Pod, clusterTopology)
	if err != nil {
		return err
	}

	return p.updateTopology(clusterTopology, topologyCm)
}
