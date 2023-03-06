package util

import (
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/common/constants"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func IsSharedGpuPod(pod *v1.Pod) bool {
	_, runaiGpuExists := pod.Annotations[constants.GpuIdxAnnotation]
	_, runaiGpuGroupExists := pod.Labels[constants.GpuGroupLabel]
	isReservationPod := pod.Namespace == constants.ReservationNs

	return !isReservationPod && (runaiGpuExists || runaiGpuGroupExists)
}

func IsDedicatedGpuPod(pod *v1.Pod) bool {
	return !pod.Spec.Containers[0].Resources.Limits[constants.GpuResourceName].Equal(resource.MustParse("0"))
}

func IsPodRunning(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodRunning
}

func IsPodTerminated(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed
}

func IsPodScheduled(pod *v1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == v1.PodScheduled && condition.Status == v1.ConditionTrue {
			return true
		}
	}

	return false
}
