package handle

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	gpuUtilizationAnnotationKey = "run.ai/simulated-gpu-utilization"
	gpuFractionAnnotationKey    = "gpu-fraction"

	idleGpuPodNamePrefix = "runai-idle-gpu-"
)

var defaultGpuUtil = topology.Range{
	Min: 100,
	Max: 100,
}

func calculateUsage(dynamicclient dynamic.Interface, pod *v1.Pod, totalGpuMemory int) topology.GpuUsageStatus {
	gpuFraction := 1.0
	if podGpuFractionStr, ok := pod.Annotations[gpuFractionAnnotationKey]; ok {
		if parsed, err := strconv.ParseFloat(podGpuFractionStr, 32); err == nil {
			gpuFraction = parsed
		} else {
			log.Printf("Error parsing gpu-fraction annotation: %s\n", err)
		}
	}

	if isPodNameMarkedAsIdle(pod.Name) {
		return generateGpuUsageStatus(topology.Range{Min: 0, Max: 0}, gpuFraction, totalGpuMemory, false)
	}

	podGpuUtilAnnotationStr, podGpuUtilAnnotationExists := pod.Annotations[gpuUtilizationAnnotationKey]
	if podGpuUtilAnnotationExists {
		gpuUtilization, err := calculateUtilizationFromAnnotation(podGpuUtilAnnotationStr)
		if err != nil {
			log.Printf("Error calculating GPU usage for pod %s from annotation: %s\n", pod.Name, err)
		}
		if gpuUtilization != nil {
			return generateGpuUsageStatus(*gpuUtilization, gpuFraction, totalGpuMemory, false)
		}
	}

	return calculateGpuUsageFromPodType(dynamicclient, pod, gpuFraction, totalGpuMemory)
}

func calculateGpuUsageFromPodType(dynamicclient dynamic.Interface, pod *v1.Pod, gpuFraction float64, totalGpuMemory int) topology.GpuUsageStatus {
	podType, err := getPodType(dynamicclient, pod)
	if err != nil {
		log.Printf("Error getting pod type for pod %s: %s\n", pod.Name, err)
	}

	switch podType {
	case "train":
		return generateGpuUsageStatus(topology.Range{Min: 80, Max: 100}, gpuFraction, totalGpuMemory, false)
	case "build", "interactive-preemptible":
		return generateGpuUsageStatus(topology.Range{Min: 0, Max: 0}, gpuFraction, totalGpuMemory, false)
	case "inference":
		return generateGpuUsageStatus(topology.Range{Min: 0, Max: 0}, gpuFraction, totalGpuMemory, true)
	default:
		return generateGpuUsageStatus(defaultGpuUtil, gpuFraction, totalGpuMemory, false)
	}
}

func calculateUtilizationFromAnnotation(annotationValue string) (*topology.Range, error) {
	re := regexp.MustCompile(`(\d*)-*(\d*)`)
	submatches := re.FindSubmatch([]byte(annotationValue))
	if len(submatches) < 2 {
		return nil, fmt.Errorf("annotation %s isn't valid", annotationValue)
	}

	minUtilization, err := strconv.Atoi(string(submatches[1]))
	if err != nil {
		return nil, fmt.Errorf("%s failed to parse to int: %s", submatches[1], err)
	}

	maxUtilization := minUtilization
	if len(submatches) > 2 && string(submatches[2]) != "" {
		maxUtilization, err = strconv.Atoi(string(submatches[2]))
		if err != nil {
			return nil, fmt.Errorf("%s failed to parse to intt, len %d: %s", submatches[2], len(submatches), err)
		}
	}

	return &topology.Range{Min: minUtilization, Max: maxUtilization}, nil
}

func getPodType(dynamicClient dynamic.Interface, pod *v1.Pod) (string, error) {
	podGroupName := pod.Annotations["pod-group-name"]
	if podGroupName == "" {
		return "", fmt.Errorf("pod %s has no pod-group-name annotation", pod.Name)
	}

	gvr := schema.GroupVersionResource{Group: "scheduling.run.ai", Version: "v1", Resource: "podgroups"}
	podGroup, err := dynamicClient.Resource(gvr).Namespace(pod.Namespace).Get(context.TODO(), podGroupName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("error getting podgroup %s: %v", podGroupName, err)
	}

	podGroupType, found, err := unstructured.NestedString(podGroup.Object, "spec", "priorityClassName")
	if err != nil {
		return "", fmt.Errorf("error getting podgroup class name: %v", err)
	}
	if !found {
		return "", fmt.Errorf("podgroup %s has no class name", podGroupName)
	}

	return podGroupType, nil
}

func isPodNameMarkedAsIdle(podName string) bool {
	return strings.HasPrefix(podName, idleGpuPodNamePrefix)
}

func generateGpuUsageStatus(gpuUtilization topology.Range, gpuFraction float64, totalGpuMemory int, isInferencePod bool) topology.GpuUsageStatus {
	return topology.GpuUsageStatus{
		Utilization:    gpuUtilization,
		FbUsed:         int(float64(totalGpuMemory) * gpuFraction),
		IsInferencePod: isInferencePod,
	}
}
