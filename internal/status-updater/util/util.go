package util

import (
	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
)

func IsSharedGpuPod(pod *v1.Pod) bool {
	_, runaiGpuExists := pod.Annotations[constants.AnnotationGpuIdx]
	_, runaiGpuGroupExists := pod.Labels[constants.LabelGpuGroup]

	return !IsGpuReservationPod(pod) && (runaiGpuExists || runaiGpuGroupExists)
}

func IsDedicatedGpuPod(pod *v1.Pod) bool {
	for _, container := range pod.Spec.Containers {
		if ContainerGpuLimit(&container) > 0 {
			return true
		}
	}
	return false
}

// ContainerGpuLimit returns the number of whole GPUs the container requests.
func ContainerGpuLimit(container *v1.Container) int64 {
	return container.Resources.Limits.Name(constants.GpuResourceName, resource.DecimalSI).Value()
}

func IsPodRunning(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodRunning
}

func IsPodTerminated(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed
}

func IsPodScheduled(pod *v1.Pod) bool {
	// This may be checked using the pod's PodScheduled condition once https://github.com/run-ai/runai-engine/pull/174 is merged and available.
	return pod.Spec.NodeName != ""
}

func IsGpuReservationPod(pod *v1.Pod) bool {
	resourceReservationNs := viper.GetString(constants.EnvResourceReservationNamespace)
	return pod.Namespace == resourceReservationNs
}

// IsDraPod returns true if the pod uses Dynamic Resource Allocation (DRA)
// by checking if pod.Spec.ResourceClaims is non-empty.
func IsDraPod(pod *v1.Pod) bool {
	return len(pod.Spec.ResourceClaims) > 0
}
