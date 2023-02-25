package util

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func IsSharedGpuPod(pod *v1.Pod) bool {
	_, runaiGpuExists := pod.Annotations["runai-gpu"]
	_, runaiGpuGroupExists := pod.Labels["runai-gpu-group"]
	isReservationPod := pod.Namespace == "runai-reservation"

	return !isReservationPod && (runaiGpuExists || runaiGpuGroupExists)
}

func IsDedicatedGpuPod(pod *v1.Pod) bool {
	return !pod.Spec.Containers[0].Resources.Limits["nvidia.com/gpu"].Equal(resource.MustParse("0"))
}

func IsPodRunning(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodRunning
}
