package pod

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/util"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (p *PodHandler) handleGpuReservationPodAddition(pod *v1.Pod) error {
	if !util.IsGpuReservationPod(pod) {
		return nil
	}

	err := p.setReservationPodGpuIdxAnnotation(pod)
	if err != nil {
		return fmt.Errorf("failed to set GPU index annotation for reservation pod %s: %w", pod.Name, err)
	}

	return nil
}

func (p *PodHandler) setReservationPodGpuIdxAnnotation(pod *v1.Pod) error {
	annotationKey := constants.ReservationPodGpuIdxAnnotation
	annotationVal := fmt.Sprintf("GPU-%s", uuid.NewString())
	patch := []byte(fmt.Sprintf(`{"metadata": {"annotations": {"%s": "%s"}}}`, annotationKey, annotationVal))

	_, err := p.kubeClient.CoreV1().Pods(pod.Namespace).Patch(context.TODO(), pod.Name, types.MergePatchType, patch, metav1.PatchOptions{})
	if err != nil {
		return fmt.Errorf("failed to update pod %s: %w", pod.Name, err)
	}

	return nil
}
