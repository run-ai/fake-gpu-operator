/*
 * Copyright 2023 The Kubernetes Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

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
	AnnotationGpuFakeDevices = "nvidia.com/gpu.fake.devices"
)

// GpuFakeDevicesAnnotation represents the structure of the node annotation
type GpuFakeDevicesAnnotation struct {
	Version string    `json:"version"`
	GPUs    []GpuInfo `json:"gpus"`
}

// GpuInfo represents a single GPU device from the annotation
type GpuInfo struct {
	UUID                  string `json:"uuid"`
	Minor                 int    `json:"minor"`
	ProductName           string `json:"productName"`
	Brand                 string `json:"brand"`
	Architecture          string `json:"architecture"`
	CudaComputeCapability string `json:"cudaComputeCapability"`
	MemoryBytes           int64  `json:"memoryBytes"`
	PcieBusID             string `json:"pcieBusID"`
	MigEnabled            bool   `json:"migEnabled"`
	VfioEnabled           bool   `json:"vfioEnabled"`
}

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

	// Parse JSON
	var annotation GpuFakeDevicesAnnotation
	if err := json.Unmarshal([]byte(annotationValue), &annotation); err != nil {
		return nil, fmt.Errorf("failed to parse annotation %s: %w", AnnotationGpuFakeDevices, err)
	}

	if len(annotation.GPUs) == 0 {
		return nil, fmt.Errorf("annotation %s contains no GPUs", AnnotationGpuFakeDevices)
	}

	// Map GPU info to resourceapi.Device structures
	alldevices := make(AllocatableDevices)
	for _, gpu := range annotation.GPUs {
		if gpu.UUID == "" {
			return nil, fmt.Errorf("GPU entry missing UUID in annotation")
		}

		// Use UUID as device name
		deviceName := strings.ToLower(gpu.UUID)

		attributes := map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
			"uuid": {
				StringValue: ptr.To(gpu.UUID),
			},
			"model": {
				StringValue: ptr.To(gpu.ProductName),
			},
			"minor": {
				IntValue: ptr.To(int64(gpu.Minor)),
			},
			"pcieBusID": {
				StringValue: ptr.To(gpu.PcieBusID),
			},
			"architecture": {
				StringValue: ptr.To(gpu.Architecture),
			},
			"cudaComputeCapability": {
				StringValue: ptr.To(gpu.CudaComputeCapability),
			},
			"migEnabled": {
				BoolValue: ptr.To(gpu.MigEnabled),
			},
			"vfioEnabled": {
				BoolValue: ptr.To(gpu.VfioEnabled),
			},
		}

		// Convert memoryBytes to resource.Quantity
		memoryQuantity := resource.NewQuantity(gpu.MemoryBytes, resource.BinarySI)

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

// convertToNodeTopology converts GpuFakeDevicesAnnotation to topology.NodeTopology format
func convertToNodeTopology(annotation GpuFakeDevicesAnnotation, nodeName string) (*topology.NodeTopology, error) {
	if len(annotation.GPUs) == 0 {
		return nil, fmt.Errorf("annotation contains no GPUs")
	}

	// Extract GpuMemory and GpuProduct from first GPU (assuming all GPUs have same specs)
	firstGPU := annotation.GPUs[0]
	gpuMemory := int(firstGPU.MemoryBytes / (1024 * 1024)) // Convert bytes to MB
	gpuProduct := firstGPU.ProductName

	// Convert GPUs from annotation to GpuDetails format
	gpus := make([]topology.GpuDetails, 0, len(annotation.GPUs))
	for _, gpu := range annotation.GPUs {
		gpuDetails := topology.GpuDetails{
			ID: gpu.UUID,
			Status: topology.GpuStatus{
				AllocatedBy: topology.ContainerDetails{
					// Empty initially - will be populated when allocated
					Namespace: "",
					Pod:       "",
					Container: "",
				},
				PodGpuUsageStatus: make(topology.PodGpuUsageStatusMap),
			},
		}
		gpus = append(gpus, gpuDetails)
	}

	nodeTopology := &topology.NodeTopology{
		GpuMemory:   gpuMemory,
		GpuProduct:  gpuProduct,
		Gpus:        gpus,
		MigStrategy: "none", // Default, can be updated if needed
	}

	return nodeTopology, nil
}
