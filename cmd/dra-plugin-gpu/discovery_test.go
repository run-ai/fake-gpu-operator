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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnumerateAllPossibleDevices(t *testing.T) {
	tests := map[string]struct {
		nodeName        string
		annotation      string
		setupNode       func(*corev1.Node)
		wantErr         bool
		wantDeviceCount int
		validateDevice  func(*testing.T, AllocatableDevices)
	}{
		"success single GPU": {
			nodeName: "test-node",
			annotation: `{
				"version": "v1",
				"gpus": [
					{
						"uuid": "GPU-12345678-1234-1234-1234-123456789abc",
						"minor": 0,
						"productName": "NVIDIA-A100-SXM4-40GB",
						"brand": "NVIDIA",
						"architecture": "Ampere",
						"cudaComputeCapability": "8.0",
						"memoryBytes": 42949672960,
						"pcieBusID": "0000:00:1E.0",
						"migEnabled": false,
						"vfioEnabled": true
					}
				]
			}`,
			wantErr:         false,
			wantDeviceCount: 1,
			validateDevice: func(t *testing.T, devices AllocatableDevices) {
				require.Len(t, devices, 1)
				device, exists := devices["gpu-12345678-1234-1234-1234-123456789abc"]
				require.True(t, exists)
				assert.Equal(t, "gpu-12345678-1234-1234-1234-123456789abc", device.Name)
				assert.Equal(t, "NVIDIA-A100-SXM4-40GB", device.Attributes["model"].StringValue)
				assert.Equal(t, int64(0), *device.Attributes["minor"].IntValue)
			},
		},
		"success multiple GPUs": {
			nodeName: "test-node",
			annotation: `{
				"version": "v1",
				"gpus": [
					{
						"uuid": "GPU-11111111-1111-1111-1111-111111111111",
						"minor": 0,
						"productName": "GPU-1",
						"memoryBytes": 1000000000
					},
					{
						"uuid": "GPU-22222222-2222-2222-2222-222222222222",
						"minor": 1,
						"productName": "GPU-2",
						"memoryBytes": 2000000000
					}
				]
			}`,
			wantErr:         false,
			wantDeviceCount: 2,
		},
		"node not found": {
			nodeName:   "non-existent-node",
			annotation: `{"version": "v1", "gpus": []}`,
			wantErr:    true,
		},
		"annotation missing": {
			nodeName: "test-node",
			setupNode: func(node *corev1.Node) {
				node.Annotations = nil
			},
			wantErr: true,
		},
		"annotation empty": {
			nodeName: "test-node",
			setupNode: func(node *corev1.Node) {
				node.Annotations[AnnotationGpuFakeDevices] = ""
			},
			wantErr: true,
		},
		"invalid JSON": {
			nodeName: "test-node",
			setupNode: func(node *corev1.Node) {
				node.Annotations[AnnotationGpuFakeDevices] = "{invalid json}"
			},
			wantErr: true,
		},
		"empty GPUs array": {
			nodeName:   "test-node",
			annotation: `{"version": "v1", "gpus": []}`,
			wantErr:    true,
		},
		"GPU missing UUID": {
			nodeName: "test-node",
			annotation: `{
				"version": "v1",
				"gpus": [
					{
						"minor": 0,
						"productName": "GPU-1",
						"memoryBytes": 1000000000
					}
				]
			}`,
			wantErr: true,
		},
		"minimal GPU fields": {
			nodeName: "test-node",
			annotation: `{
				"version": "v1",
				"gpus": [
					{
						"uuid": "GPU-minimal",
						"memoryBytes": 1000000000
					}
				]
			}`,
			wantErr:         false,
			wantDeviceCount: 1,
			validateDevice: func(t *testing.T, devices AllocatableDevices) {
				device := devices["gpu-minimal"]
				assert.Equal(t, "gpu-minimal", device.Name)
				// Check that optional fields are handled gracefully
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			client := fake.NewSimpleClientset()

			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        test.nodeName,
					Annotations: map[string]string{},
				},
			}

			if test.annotation != "" {
				node.Annotations[AnnotationGpuFakeDevices] = test.annotation
			}

			if test.setupNode != nil {
				test.setupNode(node)
			}

			_, err := client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
			require.NoError(t, err)

			devices, err := enumerateAllPossibleDevices(ctx, client, test.nodeName)

			if test.wantErr {
				assert.Error(t, err)
				assert.Nil(t, devices)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, devices)
				assert.Len(t, devices, test.wantDeviceCount)
				if test.validateDevice != nil {
					test.validateDevice(t, devices)
				}
			}
		})
	}
}

func TestEnumerateAllPossibleDevices_DeviceAttributes(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()

	annotation := GpuFakeDevicesAnnotation{
		Version: "v1",
		GPUs: []GpuInfo{
			{
				UUID:                  "GPU-test",
				Minor:                 5,
				ProductName:           "Test-GPU",
				Brand:                 "NVIDIA",
				Architecture:          "Ampere",
				CudaComputeCapability: "8.0",
				MemoryBytes:           42949672960,
				PcieBusID:             "0000:00:1E.0",
				MigEnabled:            true,
				VfioEnabled:           false,
			},
		},
	}

	annotationJSON, err := json.Marshal(annotation)
	require.NoError(t, err)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				AnnotationGpuFakeDevices: string(annotationJSON),
			},
		},
	}

	_, err = client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
	require.NoError(t, err)

	devices, err := enumerateAllPossibleDevices(ctx, client, "test-node")
	require.NoError(t, err)
	require.Len(t, devices, 1)

	device := devices["gpu-test"]
	assert.Equal(t, "gpu-test", device.Name)
	assert.Equal(t, "Test-GPU", *device.Attributes["model"].StringValue)
	assert.Equal(t, int64(5), *device.Attributes["minor"].IntValue)
	assert.Equal(t, "0000:00:1E.0", *device.Attributes["pcieBusID"].StringValue)
	assert.Equal(t, "Ampere", *device.Attributes["architecture"].StringValue)
	assert.Equal(t, "8.0", *device.Attributes["cudaComputeCapability"].StringValue)
	assert.Equal(t, true, *device.Attributes["migEnabled"].BoolValue)
	assert.Equal(t, false, *device.Attributes["vfioEnabled"].BoolValue)
	// Verify memory capacity is set correctly
	memoryCapacity := device.Capacity["memory"].Value
	assert.Equal(t, int64(42949672960), memoryCapacity.Value())
}
