package dra_plugin_gpu

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreclientset "k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

const (
	// AnnotationGpuFakeDevices is the annotation key for GPU fake devices on nodes
	AnnotationGpuFakeDevices = "nvidia.com/gpu.fake.devices"
)

func enumerateAllPossibleDevices(ctx context.Context, coreclient coreclientset.Interface, nodeName string) (AllocatableDevices, error) {
	// Fetch the node
	node, err := coreclient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	// Read the annotation
	annotationValue, exists := node.Annotations[AnnotationGpuFakeDevices]
	if !exists {
		return nil, fmt.Errorf("annotation %s not found on node %s", AnnotationGpuFakeDevices, nodeName)
	}

	if annotationValue == "" {
		return nil, fmt.Errorf("annotation %s is empty on node %s", AnnotationGpuFakeDevices, nodeName)
	}

	// Parse JSON as NodeTopology
	var nodeTopology topology.NodeTopology
	if err := json.Unmarshal([]byte(annotationValue), &nodeTopology); err != nil {
		return nil, fmt.Errorf("failed to parse annotation %s: %w", AnnotationGpuFakeDevices, err)
	}

	if len(nodeTopology.Gpus) == 0 {
		return nil, fmt.Errorf("annotation %s contains no GPUs", AnnotationGpuFakeDevices)
	}

	// Convert GpuMemory from MB to bytes for resource.Quantity
	memoryBytes := int64(nodeTopology.GpuMemory) * 1024 * 1024

	// Map GPU info to resourceapi.Device structures
	alldevices := make(AllocatableDevices)
	for _, gpu := range nodeTopology.Gpus {
		if gpu.ID == "" {
			return nil, fmt.Errorf("GPU entry missing ID in annotation")
		}

		// Use ID (UUID) as device name, convert to lowercase for RFC 1123 compliance
		deviceName := strings.ToLower(gpu.ID)

		attributes := map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
			"uuid": {
				StringValue: ptr.To(gpu.ID),
			},
			"model": {
				StringValue: ptr.To(nodeTopology.GpuProduct),
			},
		}

		// Convert memory to resource.Quantity
		memoryQuantity := resource.NewQuantity(memoryBytes, resource.BinarySI)

		device := resourceapi.Device{
			Name:       deviceName,
			Attributes: attributes,
			Capacity: map[resourceapi.QualifiedName]resourceapi.DeviceCapacity{
				"memory": {
					Value: *memoryQuantity,
				},
			},
		}
		alldevices[device.Name] = device
	}

	return alldevices, nil
}
