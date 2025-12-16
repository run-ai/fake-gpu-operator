package resourceslice

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"

	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

const (
	DriverName = "gpu.nvidia.com"
)

// Interface defines the operations for handling ResourceSlices
type Interface interface {
	HandleAddOrUpdate(cm *corev1.ConfigMap) error
	HandleDelete(nodeName string) error
}

// ResourceSliceHandler handles ResourceSlice operations for KWOK nodes
type ResourceSliceHandler struct {
	kubeClient      kubernetes.Interface
	clusterTopology *topology.ClusterTopology
}

var _ Interface = &ResourceSliceHandler{}

// NewResourceSliceHandler creates a new ResourceSliceHandler
func NewResourceSliceHandler(kubeClient kubernetes.Interface, clusterTopology *topology.ClusterTopology) *ResourceSliceHandler {
	return &ResourceSliceHandler{
		kubeClient:      kubeClient,
		clusterTopology: clusterTopology,
	}
}

// HandleAddOrUpdate handles ConfigMap additions and updates by creating/updating the ResourceSlice
func (h *ResourceSliceHandler) HandleAddOrUpdate(cm *corev1.ConfigMap) error {
	nodeName := cm.Labels[constants.LabelTopologyCMNodeName]
	if nodeName == "" {
		return fmt.Errorf("ConfigMap %s/%s is missing node name label", cm.Namespace, cm.Name)
	}

	log.Printf("Handling ConfigMap add/update for KWOK node: %s\n", nodeName)

	nodeTopology, err := topology.FromNodeTopologyCM(cm)
	if err != nil {
		return fmt.Errorf("failed to read node topology from ConfigMap: %w", err)
	}

	return h.createOrUpdateResourceSlice(nodeName, nodeTopology)
}

// HandleDelete handles ConfigMap deletions by deleting the ResourceSlice
func (h *ResourceSliceHandler) HandleDelete(nodeName string) error {
	log.Printf("Handling ConfigMap deletion for KWOK node: %s\n", nodeName)
	return h.deleteResourceSlice(nodeName)
}

func (h *ResourceSliceHandler) createOrUpdateResourceSlice(nodeName string, nodeTopology *topology.NodeTopology) error {
	devices := h.devicesFromTopology(nodeTopology)

	resourceSlice := &resourceapi.ResourceSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: h.resourceSliceName(nodeName),
		},
		Spec: resourceapi.ResourceSliceSpec{
			Driver:   DriverName,
			NodeName: ptr.To(nodeName),
			Pool: resourceapi.ResourcePool{
				Name:               nodeName,
				ResourceSliceCount: 1,
			},
			Devices: devices,
		},
	}

	// Try to get existing ResourceSlice
	existing, err := h.kubeClient.ResourceV1().ResourceSlices().Get(context.TODO(), resourceSlice.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new ResourceSlice
			_, err = h.kubeClient.ResourceV1().ResourceSlices().Create(context.TODO(), resourceSlice, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create ResourceSlice for node %s: %w", nodeName, err)
			}
			log.Printf("Created ResourceSlice for KWOK node %s with %d devices\n", nodeName, len(devices))
			return nil
		}
		return fmt.Errorf("failed to get ResourceSlice for node %s: %w", nodeName, err)
	}

	// Update existing ResourceSlice
	resourceSlice.ResourceVersion = existing.ResourceVersion
	_, err = h.kubeClient.ResourceV1().ResourceSlices().Update(context.TODO(), resourceSlice, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update ResourceSlice for node %s: %w", nodeName, err)
	}
	log.Printf("Updated ResourceSlice for KWOK node %s with %d devices\n", nodeName, len(devices))
	return nil
}

func (h *ResourceSliceHandler) deleteResourceSlice(nodeName string) error {
	err := h.kubeClient.ResourceV1().ResourceSlices().Delete(context.TODO(), h.resourceSliceName(nodeName), metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ResourceSlice for node %s: %w", nodeName, err)
	}
	log.Printf("Deleted ResourceSlice for KWOK node %s\n", nodeName)
	return nil
}

func (h *ResourceSliceHandler) resourceSliceName(nodeName string) string {
	return fmt.Sprintf("kwok-%s-gpu", nodeName)
}

func (h *ResourceSliceHandler) devicesFromTopology(nodeTopology *topology.NodeTopology) []resourceapi.Device {
	devices := make([]resourceapi.Device, 0, len(nodeTopology.Gpus))

	// Convert GpuMemory from MB to bytes for resource.Quantity
	memoryBytes := int64(nodeTopology.GpuMemory) * 1024 * 1024

	for _, gpu := range nodeTopology.Gpus {
		if gpu.ID == "" {
			log.Printf("Warning: GPU entry missing ID in topology, skipping")
			continue
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
		devices = append(devices, device)
	}

	return devices
}
