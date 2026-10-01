package pod

import (
	"fmt"
	"log"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/util"
	v1 "k8s.io/api/core/v1"
)

func (p *PodHandler) handleDedicatedGpuPodAddition(pod *v1.Pod, nodeTopology *topology.NodeTopology) error {
	if !util.IsDedicatedGpuPod(pod) {
		return nil
	}

	// This can happen when the status updater is restarted.
	// (If that will affect performance, we should construct a helper map of allocated pods)
	if isAlreadyAllocated(pod, nodeTopology) {
		log.Printf("Pod %s is already allocated, skipping...\n", pod.Name)
		return nil
	}

	for _, container := range pod.Spec.Containers {
		requestedGpusCount := util.ContainerGpuLimit(&container)
		if requestedGpusCount <= 0 {
			continue
		}

		log.Printf("Container %s requested GPUs: %d\n", container.Name, requestedGpusCount)
		for idx := range nodeTopology.Gpus {
			gpu := &nodeTopology.Gpus[idx]

			if requestedGpusCount <= 0 {
				break
			}

			if gpu.Status.AllocatedBy.Pod == "" {
				log.Printf("GPU %s is free, allocating...\n", gpu.ID)
				gpu.Status.AllocatedBy.Namespace = pod.Namespace
				gpu.Status.AllocatedBy.Pod = pod.Name
				gpu.Status.AllocatedBy.Container = container.Name

				if !util.IsGpuReservationPod(pod) {
					gpu.Status.PodGpuUsageStatus[pod.UID] = calculateUsage(p.dynamicClient, pod, nodeTopology.GpuMemory)
				}

				requestedGpusCount--
			}
		}
	}

	err := p.handleGpuReservationPodAddition(pod, nodeTopology)
	if err != nil {
		return fmt.Errorf("failed to handle GPU reservation pod addition: %w", err)
	}

	return nil
}

func (p *PodHandler) handleDedicatedGpuPodUpdate(pod *v1.Pod, nodeTopology *topology.NodeTopology) error {
	if !util.IsDedicatedGpuPod(pod) {
		return nil
	}

	for idx := range nodeTopology.Gpus {
		gpu := &nodeTopology.Gpus[idx]

		if isGpuOccupiedByPod(gpu, pod) {
			if !util.IsGpuReservationPod(pod) {
				gpu.Status.PodGpuUsageStatus[pod.UID] =
					calculateUsage(p.dynamicClient, pod, nodeTopology.GpuMemory)
			}
		}
	}

	return nil
}

func (p *PodHandler) handleDedicatedGpuPodDeletion(pod *v1.Pod, nodeTopology *topology.NodeTopology) {
	if !util.IsDedicatedGpuPod(pod) {
		return
	}

	for idx := range nodeTopology.Gpus {
		if isGpuOccupiedByPod(&nodeTopology.Gpus[idx], pod) {
			nodeTopology.Gpus[idx].Status = topology.GpuStatus{}
		}
	}
}

func isAlreadyAllocated(pod *v1.Pod, nodeTopology *topology.NodeTopology) bool {
	for idx := range nodeTopology.Gpus {
		if isGpuOccupiedByPod(&nodeTopology.Gpus[idx], pod) {
			return true
		}
	}

	return false
}

func isGpuOccupiedByPod(gpu *topology.GpuDetails, pod *v1.Pod) bool {
	return gpu.Status.AllocatedBy.Namespace == pod.Namespace &&
		gpu.Status.AllocatedBy.Pod == pod.Name
}
