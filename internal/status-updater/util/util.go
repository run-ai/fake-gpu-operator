package util

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func IsSharedGpuPod(pod *v1.Pod) bool {
	_, ok := pod.Annotations["runai-gpu"]
	return ok
}

func IsDedicatedGpuPod(pod *v1.Pod) bool {
	return !pod.Spec.Containers[0].Resources.Limits["nvidia.com/gpu"].Equal(resource.MustParse("0"))
}
