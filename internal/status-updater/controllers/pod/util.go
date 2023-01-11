package pod

import (
	v1 "k8s.io/api/core/v1"
)

func isPodRunning(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodRunning
}
