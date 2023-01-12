package pod

import (
	"fmt"
	"log"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/util"
	v1 "k8s.io/api/core/v1"
)

func (p *PodHandler) handleDedicatedGpuPodAddition(pod *v1.Pod, clusterTopology *topology.Cluster) error {
	if !util.IsDedicatedGpuPod(pod) {
		return nil
	}

	requestedGpus := pod.Spec.Containers[0].Resources.Limits.Name("nvidia.com/gpu", "")
	if requestedGpus == nil {
		return fmt.Errorf("no GPUs requested in pod %s", pod.Name)
	}

	requestedGpusCount := requestedGpus.Value()
	log.Printf("Requested GPUs: %d\n", requestedGpusCount)
	for idx, gpu := range clusterTopology.Nodes[pod.Spec.NodeName].Gpus {
		if requestedGpusCount <= 0 {
			break
		}

		if gpu.Status.AllocatedBy.Pod == "" {
			log.Printf("GPU %s is free, allocating...\n", gpu.ID)
			gpu.Status.AllocatedBy.Namespace = pod.Namespace
			gpu.Status.AllocatedBy.Pod = pod.Name
			gpu.Status.AllocatedBy.Container = pod.Spec.Containers[0].Name

			clusterTopology.Nodes[pod.Spec.NodeName].Gpus[idx] = gpu

			if pod.Namespace != "runai-reservation" {
				gpu.Status.PodGpuUsageStatus[pod.UID] = calculateUsage(p.dynamicClient, pod, clusterTopology.Nodes[pod.Spec.NodeName].GpuMemory)
			}

			requestedGpusCount--
		}
	}

	return nil
}

func (p *PodHandler) handleDedicatedGpuPodDeletion(pod *v1.Pod, clusterTopology *topology.Cluster) {
	for idx, gpu := range clusterTopology.Nodes[pod.Spec.NodeName].Gpus {
		isGpuOccupiedByPod := gpu.Status.AllocatedBy.Namespace == pod.Namespace &&
			gpu.Status.AllocatedBy.Pod == pod.Name &&
			gpu.Status.AllocatedBy.Container == pod.Spec.Containers[0].Name
		if isGpuOccupiedByPod {
			clusterTopology.Nodes[pod.Spec.NodeName].Gpus[idx].Status = topology.GpuStatus{}
		}
	}
}
