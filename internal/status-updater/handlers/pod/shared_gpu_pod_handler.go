package pod

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/hashicorp/go-multierror"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/util"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	runaiReservationNs = constants.ReservationNs
)

func (p *PodHandler) handleSharedGpuPodAddition(pod *v1.Pod, clusterTopology *topology.Cluster) error {
	if !util.IsSharedGpuPod(pod) {
		return nil
	}

	nodeTopology, ok := clusterTopology.Nodes[pod.Spec.NodeName]
	if !ok {
		return fmt.Errorf("could not find node %s in cluster topology", pod.Spec.NodeName)
	}

	reservationPodGpuIdx, err := getMatchingReservationPodGpuIdx(p.kubeClient, pod, &nodeTopology)
	if err != nil {
		return err
	}

	nodeTopology.Gpus[reservationPodGpuIdx].Status.PodGpuUsageStatus[pod.UID] = calculateUsage(p.dynamicClient, pod, nodeTopology.GpuMemory)
	return nil
}

func (p *PodHandler) handleSharedGpuPodDeletion(pod *v1.Pod, clusterTopology *topology.Cluster) error {
	if !util.IsSharedGpuPod(pod) {
		return nil
	}

	nodeTopology, ok := clusterTopology.Nodes[pod.Spec.NodeName]
	if !ok {
		return fmt.Errorf("could not find node %s in cluster topology", pod.Spec.NodeName)
	}

	reservationPodGpuIdx, err := getMatchingReservationPodGpuIdx(p.kubeClient, pod, &nodeTopology)
	if err != nil {
		return err
	}

	delete(nodeTopology.Gpus[reservationPodGpuIdx].Status.PodGpuUsageStatus, pod.UID)
	return nil
}

func getMatchingReservationPodGpuIdx(kubeclient kubernetes.Interface, pod *v1.Pod, nodeTopology *topology.Node) (int, error) {
	reservationPodName, err := getMatchingReservationPodName(kubeclient, pod)
	if err != nil {
		return -1, err
	}

	if reservationPodName == "" {
		log.Printf("Empty reservation pod name for pod %s\n", pod.Name)
		return -1, nil
	}

	reservationPodGpuIdx := -1
	for gpuIdx, gpuDetails := range nodeTopology.Gpus {
		if gpuDetails.Status.AllocatedBy.Pod == reservationPodName {
			reservationPodGpuIdx = gpuIdx
			break
		}
	}

	if reservationPodGpuIdx == -1 {
		return -1, fmt.Errorf("could not find reservation pod %s in node %s in cluster topology", reservationPodName, pod.Spec.NodeName)
	}

	return reservationPodGpuIdx, nil
}

func getMatchingReservationPodName(kubeclient kubernetes.Interface, pod *v1.Pod) (string, error) {
	var multiErr error

	// DEPRECATED_START
	reservationPodName, err := getMatchingReservationPodNameByRunaiGpuAnnotation(kubeclient, pod)
	if err == nil {
		return reservationPodName, nil
	} else {
		multiErr = multierror.Append(multiErr, fmt.Errorf("failed to find reservation pod by runai-gpu annotation: %v", err))
	}
	// DEPRECATED_END

	reservationPodName, err = getMatchingReservationPodNameByRunaiGpuGroupLabel(kubeclient, pod)
	if err == nil {
		return reservationPodName, nil
	} else {
		multiErr = multierror.Append(multiErr, fmt.Errorf("failed to find reservation pod by runai-gpu-group label: %v", err))
	}

	return "", multiErr
}

func getMatchingReservationPodNameByRunaiGpuAnnotation(kubeclient kubernetes.Interface, pod *v1.Pod) (string, error) {
	runaiGpu := pod.Annotations[constants.GpuIdxAnnotation]
	if runaiGpu == "" {
		return "", fmt.Errorf("pod %s has empty runai-gpu annotation", pod.Name)
	}

	gpuIdx, err := strconv.Atoi(runaiGpu)
	if err != nil {
		return "", err
	}

	nodeReservationPods, err := getNodeReservationPods(kubeclient, pod.Spec.NodeName)
	if err != nil {
		return "", err
	}

	for _, nodeReservationPod := range nodeReservationPods.Items {
		if nodeReservationPod.Annotations[constants.ReservationPodGpuIdxAnnotation] == runaiGpu {
			return nodeReservationPod.Name, nil
		}
	}

	return "", fmt.Errorf("no reservation pod found for gpu %d on node %s", gpuIdx, pod.Spec.NodeName)
}

func getMatchingReservationPodNameByRunaiGpuGroupLabel(kubeclient kubernetes.Interface, pod *v1.Pod) (string, error) {
	runaiGpuGroup := pod.Labels[constants.GpuGroupLabel]
	if runaiGpuGroup == "" {
		return "", fmt.Errorf("pod %s has empty runai-gpu-group label", pod.Name)
	}

	nodeReservationPods, err := getNodeReservationPods(kubeclient, pod.Spec.NodeName)
	if err != nil {
		return "", err
	}

	for _, nodeReservationPod := range nodeReservationPods.Items {
		if nodeReservationPod.Labels[constants.GpuGroupLabel] == runaiGpuGroup {
			return nodeReservationPod.Name, nil
		}
	}

	return "", fmt.Errorf("no reservation pod found for gpu group %s on node %s", runaiGpuGroup, pod.Spec.NodeName)
}

func getNodeReservationPods(kubeclient kubernetes.Interface, nodeName string) (*v1.PodList, error) {
	return kubeclient.CoreV1().Pods(runaiReservationNs).List(context.TODO(), metav1.ListOptions{FieldSelector: "spec.nodeName=" + nodeName})
}
