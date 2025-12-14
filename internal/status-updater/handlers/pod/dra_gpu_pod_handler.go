package pod

import (
	"context"
	"fmt"
	"log"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/util"
	v1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DRA driver name for GPU devices
const draDriverName = "gpu.nvidia.com"

// handleDraGpuPodAddition handles GPU allocation for pods using DRA (Dynamic Resource Allocation).
// It reads the ResourceClaim allocation to get exact GPU device IDs and marks them as allocated.
func (p *PodHandler) handleDraGpuPodAddition(pod *v1.Pod, nodeTopology *topology.NodeTopology) error {
	if !util.IsDraPod(pod) {
		return nil
	}

	// Check if already allocated (for restart scenarios)
	if isDraAlreadyAllocated(pod, nodeTopology) {
		log.Printf("DRA pod %s is already allocated, skipping...\n", pod.Name)
		return nil
	}

	// Get allocated device names from ResourceClaims
	deviceNames, err := p.getDeviceNamesFromClaims(pod)
	if err != nil {
		return fmt.Errorf("failed to get device names from claims for pod %s: %w", pod.Name, err)
	}

	if len(deviceNames) == 0 {
		log.Printf("DRA pod %s has no allocated devices yet\n", pod.Name)
		return nil
	}

	log.Printf("DRA pod %s has %d allocated devices: %v\n", pod.Name, len(deviceNames), deviceNames)

	// Mark the specific GPUs as allocated
	for _, deviceName := range deviceNames {
		gpuIdx := findGpuIndexByID(nodeTopology, deviceName)
		if gpuIdx == -1 {
			log.Printf("GPU device %s not found in node topology for DRA pod %s\n", deviceName, pod.Name)
			continue
		}

		gpu := &nodeTopology.Gpus[gpuIdx]
		if gpu.Status.AllocatedBy.Pod == "" {
			log.Printf("DRA: GPU %s is free, allocating to pod %s...\n", gpu.ID, pod.Name)
			gpu.Status.AllocatedBy.Namespace = pod.Namespace
			gpu.Status.AllocatedBy.Pod = pod.Name
			// Use first container that has a claim reference
			gpu.Status.AllocatedBy.Container = getContainerWithClaim(pod)

			if gpu.Status.PodGpuUsageStatus == nil {
				gpu.Status.PodGpuUsageStatus = make(topology.PodGpuUsageStatusMap)
			}
			gpu.Status.PodGpuUsageStatus[pod.UID] = calculateUsage(p.dynamicClient, pod, nodeTopology.GpuMemory)
		} else {
			log.Printf("DRA: GPU %s is already allocated by pod %s/%s\n", gpu.ID, gpu.Status.AllocatedBy.Namespace, gpu.Status.AllocatedBy.Pod)
		}
	}

	return nil
}

// handleDraGpuPodUpdate handles GPU usage updates for pods using DRA.
func (p *PodHandler) handleDraGpuPodUpdate(pod *v1.Pod, nodeTopology *topology.NodeTopology) error {
	if !util.IsDraPod(pod) {
		return nil
	}

	// Update usage status for GPUs allocated to this pod
	for idx := range nodeTopology.Gpus {
		gpu := &nodeTopology.Gpus[idx]

		isGpuOccupiedByPod := gpu.Status.AllocatedBy.Namespace == pod.Namespace &&
			gpu.Status.AllocatedBy.Pod == pod.Name
		if isGpuOccupiedByPod {
			if gpu.Status.PodGpuUsageStatus == nil {
				gpu.Status.PodGpuUsageStatus = make(topology.PodGpuUsageStatusMap)
			}
			gpu.Status.PodGpuUsageStatus[pod.UID] = calculateUsage(p.dynamicClient, pod, nodeTopology.GpuMemory)
		}
	}

	return nil
}

// handleDraGpuPodDeletion handles GPU release for pods using DRA.
func (p *PodHandler) handleDraGpuPodDeletion(pod *v1.Pod, nodeTopology *topology.NodeTopology) {
	if !util.IsDraPod(pod) {
		return
	}

	for idx, gpu := range nodeTopology.Gpus {
		isGpuOccupiedByPod := gpu.Status.AllocatedBy.Namespace == pod.Namespace &&
			gpu.Status.AllocatedBy.Pod == pod.Name
		if isGpuOccupiedByPod {
			log.Printf("DRA: Releasing GPU %s from pod %s\n", gpu.ID, pod.Name)
			nodeTopology.Gpus[idx].Status = topology.GpuStatus{}
		}
	}
}

// getDeviceNamesFromClaims retrieves allocated device names from the pod's ResourceClaims.
func (p *PodHandler) getDeviceNamesFromClaims(pod *v1.Pod) ([]string, error) {
	claimNames := getResourceClaimNamesFromPod(pod)
	if len(claimNames) == 0 {
		return nil, nil
	}

	var deviceNames []string
	for _, claimName := range claimNames {
		claim, err := p.kubeClient.ResourceV1().ResourceClaims(pod.Namespace).Get(
			context.TODO(), claimName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to get ResourceClaim %s: %w", claimName, err)
		}

		devices := getDevicesFromClaim(claim)
		deviceNames = append(deviceNames, devices...)
	}

	return deviceNames, nil
}

// getResourceClaimNamesFromPod extracts ResourceClaim names from a pod.
// It checks pod.Status.ResourceClaimStatuses first (for template-based claims),
// then falls back to pod.Spec.ResourceClaims (for direct references).
func getResourceClaimNamesFromPod(pod *v1.Pod) []string {
	claimNames := make([]string, 0)
	seen := make(map[string]bool)

	// First, check pod.Status.ResourceClaimStatuses for generated claims from templates
	for _, status := range pod.Status.ResourceClaimStatuses {
		if status.ResourceClaimName != nil && *status.ResourceClaimName != "" {
			if !seen[*status.ResourceClaimName] {
				claimNames = append(claimNames, *status.ResourceClaimName)
				seen[*status.ResourceClaimName] = true
			}
		}
	}

	// Then, check pod.Spec.ResourceClaims for direct references
	for _, claim := range pod.Spec.ResourceClaims {
		if claim.ResourceClaimName != nil && *claim.ResourceClaimName != "" {
			if !seen[*claim.ResourceClaimName] {
				claimNames = append(claimNames, *claim.ResourceClaimName)
				seen[*claim.ResourceClaimName] = true
			}
		}
	}

	return claimNames
}

// getDevicesFromClaim extracts device names from a ResourceClaim's allocation.
func getDevicesFromClaim(claim *resourceapi.ResourceClaim) []string {
	if claim.Status.Allocation == nil {
		return nil
	}

	var devices []string
	for _, result := range claim.Status.Allocation.Devices.Results {
		// Only include devices from our GPU driver
		if result.Driver == draDriverName {
			devices = append(devices, result.Device)
		}
	}

	return devices
}

// findGpuIndexByID finds the index of a GPU in the topology by its ID.
func findGpuIndexByID(nodeTopology *topology.NodeTopology, gpuID string) int {
	for idx, gpu := range nodeTopology.Gpus {
		if gpu.ID == gpuID {
			return idx
		}
	}
	return -1
}

// isDraAlreadyAllocated checks if a DRA pod's GPUs are already allocated in the topology.
func isDraAlreadyAllocated(pod *v1.Pod, nodeTopology *topology.NodeTopology) bool {
	for _, gpu := range nodeTopology.Gpus {
		isGpuOccupiedByPod := gpu.Status.AllocatedBy.Namespace == pod.Namespace &&
			gpu.Status.AllocatedBy.Pod == pod.Name
		if isGpuOccupiedByPod {
			return true
		}
	}
	return false
}

// getContainerWithClaim returns the name of the first container that has a resource claim.
// Note: Multiple containers can share the same ResourceClaim in DRA (e.g., time-slicing scenarios),
// but topology.ContainerDetails only supports a single Container string field, so we track the first one.
func getContainerWithClaim(pod *v1.Pod) string {
	for _, container := range pod.Spec.Containers {
		if len(container.Resources.Claims) > 0 {
			return container.Name
		}
	}
	// Fallback to first container
	if len(pod.Spec.Containers) > 0 {
		return pod.Spec.Containers[0].Name
	}
	return ""
}
