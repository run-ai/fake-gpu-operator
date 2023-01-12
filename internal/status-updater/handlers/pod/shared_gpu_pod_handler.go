package pod

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/util"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	runaiReservationNs = "runai-reservation"
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
	runaiGpu := pod.Annotations["runai-gpu"]
	if runaiGpu == "" {
		return "", fmt.Errorf("pod %s has empty runai-gpu annotation", pod.Name)
	}

	gpuIdx, err := strconv.Atoi(runaiGpu)
	if err != nil {
		return "", err
	}

	nodeName := pod.Spec.NodeName

	nodeReservationPods, err := kubeclient.CoreV1().Pods(runaiReservationNs).List(context.TODO(), metav1.ListOptions{FieldSelector: "spec.nodeName=" + nodeName})
	if err != nil {
		return "", err
	}

	var matchingReservationPod *v1.Pod
	for _, nodeReservationPod := range nodeReservationPods.Items {
		if nodeReservationPod.Annotations["run.ai/reserve_for_gpu_index"] == runaiGpu {
			matchingReservationPod = &nodeReservationPod
		}
	}

	if matchingReservationPod == nil {
		return "", fmt.Errorf("no reservation pod found for gpu %d on node %s", gpuIdx, nodeName)
	}

	return matchingReservationPod.Name, nil
}
