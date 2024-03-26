package pod

import (
	"fmt"
	"log"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
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

	requestedGpus := pod.Spec.Containers[0].Resources.Limits.Name(constants.GpuResourceName, "")
	if requestedGpus == nil {
		return fmt.Errorf("no GPUs requested in pod %s", pod.Name)
	}

	requestedGpusCount := requestedGpus.Value()
	log.Printf("Requested GPUs: %d\n", requestedGpusCount)
	for idx := range nodeTopology.Gpus {
		gpu := &nodeTopology.Gpus[idx]

		if requestedGpusCount <= 0 {
			break
		}

		if gpu.Status.AllocatedBy.Pod == "" {
			log.Printf("GPU %s is free, allocating...\n", gpu.ID)
			gpu.Status.AllocatedBy.Namespace = pod.Namespace
			gpu.Status.AllocatedBy.Pod = pod.Name
			gpu.Status.AllocatedBy.Container = pod.Spec.Containers[0].Name

			if !util.IsGpuReservationPod(pod) {
				gpu.Status.PodGpuUsageStatus[pod.UID] = calculateUsage(p.dynamicClient, pod, nodeTopology.GpuMemory)
			}

			requestedGpusCount--
		}
	}

	return nil
}

func (p *PodHandler) handleDedicatedGpuPodUpdate(pod *v1.Pod, nodeTopology *topology.NodeTopology) error {
	if !util.IsDedicatedGpuPod(pod) {
		return nil
	}

	for idx := range nodeTopology.Gpus {
		gpu := &nodeTopology.Gpus[idx]

		isGpuOccupiedByPod := gpu.Status.AllocatedBy.Namespace == pod.Namespace &&
			gpu.Status.AllocatedBy.Pod == pod.Name &&
			gpu.Status.AllocatedBy.Container == pod.Spec.Containers[0].Name
		if isGpuOccupiedByPod {
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

	for idx, gpu := range nodeTopology.Gpus {
		isGpuOccupiedByPod := gpu.Status.AllocatedBy.Namespace == pod.Namespace &&
			gpu.Status.AllocatedBy.Pod == pod.Name &&
			gpu.Status.AllocatedBy.Container == pod.Spec.Containers[0].Name
		if isGpuOccupiedByPod {
			nodeTopology.Gpus[idx].Status = topology.GpuStatus{}
		}
	}
}

func isAlreadyAllocated(pod *v1.Pod, nodeTopology *topology.NodeTopology) bool {
	for _, gpu := range nodeTopology.Gpus {
		isGpuOccupiedByPod := gpu.Status.AllocatedBy.Namespace == pod.Namespace &&
			gpu.Status.AllocatedBy.Pod == pod.Name &&
			gpu.Status.AllocatedBy.Container == pod.Spec.Containers[0].Name
		if isGpuOccupiedByPod {
			return true
		}
	}

	return false
}
