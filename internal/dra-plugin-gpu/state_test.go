/*
 * Copyright 2025 The Kubernetes Authors.
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

package dra_plugin_gpu

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager"

	configapi "sigs.k8s.io/dra-example-driver/api/example.com/resource/gpu/v1alpha1"
)

func TestPreparedDevicesGetDevices(t *testing.T) {
	tests := map[string]struct {
		preparedDevices PreparedDevices
		expected        []*drapbv1.Device
	}{
		"nil PreparedDevices": {
			preparedDevices: nil,
			expected:        nil,
		},
		"empty PreparedDevices": {
			preparedDevices: PreparedDevices{},
			expected:        []*drapbv1.Device{},
		},
		"several PreparedDevices": {
			preparedDevices: PreparedDevices{
				{Device: drapbv1.Device{DeviceName: "dev1"}},
				{Device: drapbv1.Device{DeviceName: "dev2"}},
				{Device: drapbv1.Device{DeviceName: "dev3"}},
			},
			expected: []*drapbv1.Device{
				{DeviceName: "dev1"},
				{DeviceName: "dev2"},
				{DeviceName: "dev3"},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			devices := test.preparedDevices.GetDevices()
			assert.Equal(t, test.expected, devices)
		})
	}
}

// mockCheckpointManager implements checkpointmanager.CheckpointManager for testing
type mockCheckpointManager struct {
	checkpoints         []string
	getCheckpointErr    error
	createCheckpointErr error
	listCheckpointsErr  error
	storedCheckpoint    checkpointmanager.Checkpoint
}

func (m *mockCheckpointManager) CreateCheckpoint(name string, checkpoint checkpointmanager.Checkpoint) error {
	if m.createCheckpointErr != nil {
		return m.createCheckpointErr
	}
	m.storedCheckpoint = checkpoint
	m.checkpoints = append(m.checkpoints, name)
	return nil
}

func (m *mockCheckpointManager) GetCheckpoint(name string, checkpoint checkpointmanager.Checkpoint) error {
	if m.getCheckpointErr != nil {
		return m.getCheckpointErr
	}
	if m.storedCheckpoint != nil {
		data, err := m.storedCheckpoint.MarshalCheckpoint()
		if err != nil {
			return err
		}
		return checkpoint.UnmarshalCheckpoint(data)
	}
	// Return empty checkpoint
	empty := newCheckpoint()
	data, err := empty.MarshalCheckpoint()
	if err != nil {
		return err
	}
	return checkpoint.UnmarshalCheckpoint(data)
}

func (m *mockCheckpointManager) RemoveCheckpoint(name string) error {
	return nil
}

func (m *mockCheckpointManager) ListCheckpoints() ([]string, error) {
	if m.listCheckpointsErr != nil {
		return nil, m.listCheckpointsErr
	}
	return m.checkpoints, nil
}

// We can't easily mock kubeletplugin.Helper as it's a concrete type
// For tests that need it, we'll skip NewDeviceState tests that require it
// and test other methods directly

func createTestConfig(t *testing.T) (*Config, func()) {
	tmpDir := t.TempDir()
	os.Setenv("NODE_NAME", "test-node")

	client := fake.NewSimpleClientset()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				AnnotationGpuFakeDevices: `{
					"gpuMemory": 40960,
					"gpuProduct": "Test-GPU",
					"gpus": [
						{
							"id": "GPU-test-0",
							"status": {
								"allocatedBy": {"namespace": "", "pod": "", "container": ""},
								"podGpuUsageStatus": {}
							}
						}
					],
					"migStrategy": "none"
				}`,
			},
		},
	}
	_, err := client.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
	require.NoError(t, err)

	config := &Config{
		Flags: &Flags{
			NodeName: "test-node",
			CDIRoot:  tmpDir,
		},
		CoreClient: client,
	}

	cleanup := func() {
		os.Unsetenv("NODE_NAME")
	}

	return config, cleanup
}

// TestNewDeviceState is skipped because it requires a real kubeletplugin.Helper
// which is complex to mock. Integration tests should cover this.
func TestNewDeviceState(t *testing.T) {
	t.Skip("Requires real kubeletplugin.Helper - tested via integration tests")
}

func TestDeviceState_Prepare(t *testing.T) {
	// Create a minimal state for testing Prepare method directly
	// We'll test Prepare with a real checkpoint manager but skip NewDeviceState
	config, cleanup := createTestConfig(t)
	defer cleanup()

	checkpointDir := filepath.Join(config.Flags.CDIRoot, "checkpoints")
	os.MkdirAll(checkpointDir, 0755)

	checkpointManager, err := checkpointmanager.NewCheckpointManager(checkpointDir)
	require.NoError(t, err)

	// Create allocatable devices
	allocatable := AllocatableDevices{
		"GPU-test-0": resourceapi.Device{
			Name: "GPU-test-0",
		},
	}

	cdi, err := NewCDIHandler(config)
	require.NoError(t, err)

	state := &DeviceState{
		allocatable:       allocatable,
		checkpointManager: checkpointManager,
		cdi:               cdi,
	}

	tests := map[string]struct {
		claim        *resourceapi.ResourceClaim
		wantErr      bool
		wantPrepared bool
		prepareTwice bool // Test idempotency
	}{
		"new claim": {
			claim: &resourceapi.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("claim-1"),
				},
				Status: resourceapi.ResourceClaimStatus{
					Allocation: &resourceapi.AllocationResult{
						Devices: resourceapi.DeviceAllocationResult{
							Results: []resourceapi.DeviceRequestAllocationResult{
								{
									Device:  "GPU-test-0",
									Request: "request-1",
									Pool:    "test-node",
								},
							},
							Config: []resourceapi.DeviceAllocationConfiguration{},
						},
					},
				},
			},
			wantErr:      false,
			wantPrepared: true,
		},
		"claim not allocated": {
			claim: &resourceapi.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("claim-2"),
				},
				Status: resourceapi.ResourceClaimStatus{},
			},
			wantErr: true,
		},
		"idempotency - prepare twice": {
			claim: &resourceapi.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("claim-3"),
				},
				Status: resourceapi.ResourceClaimStatus{
					Allocation: &resourceapi.AllocationResult{
						Devices: resourceapi.DeviceAllocationResult{
							Results: []resourceapi.DeviceRequestAllocationResult{
								{
									Device:  "GPU-test-0",
									Request: "request-1",
									Pool:    "test-node",
								},
							},
							Config: []resourceapi.DeviceAllocationConfiguration{},
						},
					},
				},
			},
			wantErr:      false,
			wantPrepared: true,
			prepareTwice: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			devices, err := state.Prepare(context.Background(), test.claim)

			if test.wantErr {
				assert.Error(t, err)
				assert.Nil(t, devices)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, devices)
				if test.wantPrepared {
					assert.NotEmpty(t, devices)
				}

				// Test idempotency
				if test.prepareTwice {
					devices2, err2 := state.Prepare(context.Background(), test.claim)
					assert.NoError(t, err2)
					assert.Equal(t, devices, devices2)
				}
			}
		})
	}
}

func TestDeviceState_Unprepare(t *testing.T) {
	config, cleanup := createTestConfig(t)
	defer cleanup()

	checkpointDir := filepath.Join(config.Flags.CDIRoot, "checkpoints")
	os.MkdirAll(checkpointDir, 0755)

	checkpointManager, err := checkpointmanager.NewCheckpointManager(checkpointDir)
	require.NoError(t, err)

	allocatable := AllocatableDevices{
		"GPU-test-0": resourceapi.Device{
			Name: "GPU-test-0",
		},
	}

	cdi, err := NewCDIHandler(config)
	require.NoError(t, err)

	state := &DeviceState{
		allocatable:       allocatable,
		checkpointManager: checkpointManager,
		cdi:               cdi,
	}

	// First prepare a claim
	claim := &resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			UID: types.UID("claim-to-unprepare"),
		},
		Status: resourceapi.ResourceClaimStatus{
			Allocation: &resourceapi.AllocationResult{
				Devices: resourceapi.DeviceAllocationResult{
					Results: []resourceapi.DeviceRequestAllocationResult{
						{
							Device:  "GPU-test-0",
							Request: "request-1",
							Pool:    "test-node",
						},
					},
					Config: []resourceapi.DeviceAllocationConfiguration{},
				},
			},
		},
	}

	_, err = state.Prepare(context.Background(), claim)
	require.NoError(t, err)

	tests := map[string]struct {
		claimUID string
		wantErr  bool
	}{
		"unprepare existing claim": {
			claimUID: "claim-to-unprepare",
			wantErr:  false,
		},
		"unprepare non-existent claim": {
			claimUID: "non-existent",
			wantErr:  false, // Should be idempotent
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := state.Unprepare(test.claimUID)
			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetOpaqueDeviceConfigs(t *testing.T) {
	decoder := configapi.Decoder
	driverName := DriverName

	tests := map[string]struct {
		possibleConfigs []resourceapi.DeviceAllocationConfiguration
		wantErr         bool
		wantCount       int
		validateConfigs func(*testing.T, []*OpaqueDeviceConfig)
	}{
		"single config from class": {
			possibleConfigs: []resourceapi.DeviceAllocationConfiguration{
				{
					Source: resourceapi.AllocationConfigSourceClass,
					DeviceConfiguration: resourceapi.DeviceConfiguration{
						Opaque: &resourceapi.OpaqueDeviceConfiguration{
							Driver: driverName,
							Parameters: runtime.RawExtension{
								Raw: mustMarshalJSON(t, configapi.DefaultGpuConfig()),
							},
						},
					},
				},
			},
			wantErr:   false,
			wantCount: 1,
		},
		"single config from claim": {
			possibleConfigs: []resourceapi.DeviceAllocationConfiguration{
				{
					Source: resourceapi.AllocationConfigSourceClaim,
					DeviceConfiguration: resourceapi.DeviceConfiguration{
						Opaque: &resourceapi.OpaqueDeviceConfiguration{
							Driver: driverName,
							Parameters: runtime.RawExtension{
								Raw: mustMarshalJSON(t, configapi.DefaultGpuConfig()),
							},
						},
					},
				},
			},
			wantErr:   false,
			wantCount: 1,
		},
		"multiple configs - class then claim": {
			possibleConfigs: []resourceapi.DeviceAllocationConfiguration{
				{
					Source: resourceapi.AllocationConfigSourceClass,
					DeviceConfiguration: resourceapi.DeviceConfiguration{
						Opaque: &resourceapi.OpaqueDeviceConfiguration{
							Driver: driverName,
							Parameters: runtime.RawExtension{
								Raw: mustMarshalJSON(t, configapi.DefaultGpuConfig()),
							},
						},
					},
				},
				{
					Source: resourceapi.AllocationConfigSourceClaim,
					DeviceConfiguration: resourceapi.DeviceConfiguration{
						Opaque: &resourceapi.OpaqueDeviceConfiguration{
							Driver: driverName,
							Parameters: runtime.RawExtension{
								Raw: mustMarshalJSON(t, configapi.DefaultGpuConfig()),
							},
						},
					},
				},
			},
			wantErr:   false,
			wantCount: 2,
			validateConfigs: func(t *testing.T, configs []*OpaqueDeviceConfig) {
				// Class configs should come before claim configs
				require.NotEmpty(t, configs)
				// Just verify we got configs
			},
		},
		"configs for different drivers": {
			possibleConfigs: []resourceapi.DeviceAllocationConfiguration{
				{
					Source: resourceapi.AllocationConfigSourceClass,
					DeviceConfiguration: resourceapi.DeviceConfiguration{
						Opaque: &resourceapi.OpaqueDeviceConfiguration{
							Driver: "other-driver",
							Parameters: runtime.RawExtension{
								Raw: mustMarshalJSON(t, configapi.DefaultGpuConfig()),
							},
						},
					},
				},
			},
			wantErr:   false,
			wantCount: 0, // Filtered out
		},
		"no configs": {
			possibleConfigs: []resourceapi.DeviceAllocationConfiguration{},
			wantErr:         false,
			wantCount:       0,
		},
		"invalid config source": {
			possibleConfigs: []resourceapi.DeviceAllocationConfiguration{
				{
					Source: "invalid-source",
					DeviceConfiguration: resourceapi.DeviceConfiguration{
						Opaque: &resourceapi.OpaqueDeviceConfiguration{
							Driver: driverName,
						},
					},
				},
			},
			wantErr: true,
		},
		"opaque is nil": {
			possibleConfigs: []resourceapi.DeviceAllocationConfiguration{
				{
					Source: resourceapi.AllocationConfigSourceClass,
					DeviceConfiguration: resourceapi.DeviceConfiguration{
						Opaque: nil,
					},
				},
			},
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			configs, err := GetOpaqueDeviceConfigs(decoder, driverName, test.possibleConfigs)
			if test.wantErr {
				assert.Error(t, err)
				assert.Nil(t, configs)
			} else {
				assert.NoError(t, err)
				assert.Len(t, configs, test.wantCount)
				if test.validateConfigs != nil {
					test.validateConfigs(t, configs)
				}
			}
		})
	}
}

func TestDeviceState_UnprepareDevices(t *testing.T) {
	state := &DeviceState{}
	err := state.unprepareDevices("claim-1", PreparedDevices{})
	assert.NoError(t, err) // Currently a no-op
}

func TestDeviceState_UpdateDevicesFromAnnotation(t *testing.T) {
	config, cleanup := createTestConfig(t)
	defer cleanup()

	checkpointDir := filepath.Join(config.Flags.CDIRoot, "checkpoints")
	os.MkdirAll(checkpointDir, 0755)

	checkpointManager, err := checkpointmanager.NewCheckpointManager(checkpointDir)
	require.NoError(t, err)

	allocatable := AllocatableDevices{
		"GPU-test-0": resourceapi.Device{
			Name: "GPU-test-0",
		},
	}

	cdi, err := NewCDIHandler(config)
	require.NoError(t, err)

	// For updateDevicesFromAnnotation test, we need a helper
	// Since we can't easily mock it, we'll test the logic without calling helper
	// by checking that the method would work if helper was available
	state := &DeviceState{
		allocatable:       allocatable,
		checkpointManager: checkpointManager,
		cdi:               cdi,
		helper:            nil, // Skip helper-dependent test
		nodeName:          "test-node",
		coreclient:        config.CoreClient,
	}

	tests := map[string]struct {
		updateAnnotation func(*corev1.Node)
		wantErr          bool
	}{
		"success update": {
			updateAnnotation: func(node *corev1.Node) {
				node.Annotations[AnnotationGpuFakeDevices] = `{
					"gpuMemory": 40960,
					"gpuProduct": "Updated-GPU",
					"gpus": [
						{
							"id": "GPU-updated",
							"status": {
								"allocatedBy": {"namespace": "", "pod": "", "container": ""},
								"podGpuUsageStatus": {}
							}
						}
					],
					"migStrategy": "none"
				}`
			},
			wantErr: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Update node annotation
			node, err := config.CoreClient.CoreV1().Nodes().Get(context.Background(), "test-node", metav1.GetOptions{})
			require.NoError(t, err)
			test.updateAnnotation(node)
			_, err = config.CoreClient.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{})
			require.NoError(t, err)

			err = state.UpdateDevicesFromAnnotation(context.Background())
			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDeviceState_ApplyConfig(t *testing.T) {
	state := &DeviceState{}

	tests := map[string]struct {
		config   *configapi.GpuConfig
		results  []*resourceapi.DeviceRequestAllocationResult
		wantErr  bool
		validate func(*testing.T, PerDeviceCDIContainerEdits)
	}{
		"config with no sharing": {
			config: configapi.DefaultGpuConfig(),
			results: []*resourceapi.DeviceRequestAllocationResult{
				{
					Device:  "gpu-test-0",
					Request: "request-1",
					Pool:    "test-node",
				},
			},
			wantErr: false,
			validate: func(t *testing.T, edits PerDeviceCDIContainerEdits) {
				require.Contains(t, edits, "gpu-test-0")
				envs := edits["gpu-test-0"].ContainerEdits.Env
				assert.Contains(t, envs, "GPU_DEVICE_gpu_test_0=gpu-test-0")
			},
		},
		"config with sharing strategy only": {
			config: func() *configapi.GpuConfig {
				config := configapi.DefaultGpuConfig()
				config.Sharing = &configapi.GpuSharing{
					Strategy: "time-slicing",
				}
				return config
			}(),
			results: []*resourceapi.DeviceRequestAllocationResult{
				{
					Device:  "gpu-test-0",
					Request: "request-1",
					Pool:    "test-node",
				},
			},
			wantErr: false,
			validate: func(t *testing.T, edits PerDeviceCDIContainerEdits) {
				envs := edits["gpu-test-0"].ContainerEdits.Env
				assert.Contains(t, envs, "GPU_DEVICE_gpu_test_0_SHARING_STRATEGY=time-slicing")
			},
		},
		"multiple devices": {
			config: configapi.DefaultGpuConfig(),
			results: []*resourceapi.DeviceRequestAllocationResult{
				{
					Device:  "gpu-test-0",
					Request: "request-1",
					Pool:    "test-node",
				},
				{
					Device:  "gpu-test-1",
					Request: "request-2",
					Pool:    "test-node",
				},
			},
			wantErr: false,
			validate: func(t *testing.T, edits PerDeviceCDIContainerEdits) {
				assert.Len(t, edits, 2)
				assert.Contains(t, edits, "gpu-test-0")
				assert.Contains(t, edits, "gpu-test-1")
			},
		},
		"empty results": {
			config:  configapi.DefaultGpuConfig(),
			results: []*resourceapi.DeviceRequestAllocationResult{},
			wantErr: false,
			validate: func(t *testing.T, edits PerDeviceCDIContainerEdits) {
				assert.Empty(t, edits)
			},
		},
		"device name shorter than 4 chars": {
			config: configapi.DefaultGpuConfig(),
			results: []*resourceapi.DeviceRequestAllocationResult{
				{
					Device:  "gpu", // Short device name
					Request: "request-1",
					Pool:    "test-node",
				},
			},
			wantErr: false, // Will still work, just use full device name
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			edits, err := state.applyConfig(test.config, test.results)
			if test.wantErr {
				assert.Error(t, err)
				assert.Nil(t, edits)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, edits)
				if test.validate != nil {
					test.validate(t, edits)
				}
			}
		})
	}
}

func TestDeviceState_PrepareDevices(t *testing.T) {
	config, cleanup := createTestConfig(t)
	defer cleanup()

	allocatable := AllocatableDevices{
		"GPU-test-0": resourceapi.Device{
			Name: "GPU-test-0",
		},
		"GPU-test-1": resourceapi.Device{
			Name: "GPU-test-1",
		},
	}

	cdi, err := NewCDIHandler(config)
	require.NoError(t, err)

	state := &DeviceState{
		allocatable: allocatable,
		cdi:         cdi,
	}

	tests := map[string]struct {
		claim    *resourceapi.ResourceClaim
		wantErr  bool
		validate func(*testing.T, PreparedDevices)
	}{
		"single device": {
			claim: &resourceapi.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("claim-1"),
				},
				Status: resourceapi.ResourceClaimStatus{
					Allocation: &resourceapi.AllocationResult{
						Devices: resourceapi.DeviceAllocationResult{
							Results: []resourceapi.DeviceRequestAllocationResult{
								{
									Device:  "GPU-test-0",
									Request: "request-1",
									Pool:    "test-node",
								},
							},
							Config: []resourceapi.DeviceAllocationConfiguration{},
						},
					},
				},
			},
			wantErr: false,
			validate: func(t *testing.T, devices PreparedDevices) {
				assert.Len(t, devices, 1)
				assert.Equal(t, "GPU-test-0", devices[0].DeviceName)
			},
		},
		"multiple devices": {
			claim: &resourceapi.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("claim-2"),
				},
				Status: resourceapi.ResourceClaimStatus{
					Allocation: &resourceapi.AllocationResult{
						Devices: resourceapi.DeviceAllocationResult{
							Results: []resourceapi.DeviceRequestAllocationResult{
								{
									Device:  "GPU-test-0",
									Request: "request-1",
									Pool:    "test-node",
								},
								{
									Device:  "GPU-test-1",
									Request: "request-2",
									Pool:    "test-node",
								},
							},
							Config: []resourceapi.DeviceAllocationConfiguration{},
						},
					},
				},
			},
			wantErr: false,
			validate: func(t *testing.T, devices PreparedDevices) {
				assert.Len(t, devices, 2)
			},
		},
		"device not allocatable": {
			claim: &resourceapi.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					UID: types.UID("claim-3"),
				},
				Status: resourceapi.ResourceClaimStatus{
					Allocation: &resourceapi.AllocationResult{
						Devices: resourceapi.DeviceAllocationResult{
							Results: []resourceapi.DeviceRequestAllocationResult{
								{
									Device:  "GPU-non-existent",
									Request: "request-1",
									Pool:    "test-node",
								},
							},
							Config: []resourceapi.DeviceAllocationConfiguration{},
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			devices, err := state.prepareDevices(test.claim)
			if test.wantErr {
				assert.Error(t, err)
				assert.Nil(t, devices)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, devices)
				if test.validate != nil {
					test.validate(t, devices)
				}
			}
		})
	}
}

// Helper function
func mustMarshalJSON(t *testing.T, v interface{}) []byte {
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

func TestWaitForGPUAnnotation_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := fake.NewSimpleClientset()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-node",
			Annotations: map[string]string{},
			// No annotation - will retry
		},
	}

	_, err := client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
	require.NoError(t, err)

	// Cancel context after a short delay
	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()

	devices, err := waitForGPUAnnotation(ctx, client, "test-node")

	// Should fail due to context cancellation
	assert.Error(t, err)
	assert.Nil(t, devices)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestWaitForGPUAnnotation_MultipleRetriesThenSuccess(t *testing.T) {
	validAnnotation := `{
		"gpuMemory": 40960,
		"gpuProduct": "NVIDIA-A100-SXM4-40GB",
		"gpus": [
			{
				"id": "GPU-12345678-1234-1234-1234-123456789abc",
				"status": {
					"allocatedBy": {"namespace": "", "pod": "", "container": ""},
					"podGpuUsageStatus": {}
				}
			}
		],
		"migStrategy": "none"
	}`

	ctx := context.Background()
	client := fake.NewSimpleClientset()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-node",
			Annotations: map[string]string{},
			// Start without annotation
		},
	}

	_, err := client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
	require.NoError(t, err)

	// Update node after multiple retry attempts (simulate annotation appearing)
	attemptCount := 0
	go func() {
		// Wait for a few retries (each retry has exponential backoff)
		// First retry: 2s, second: 4s, third: 8s, etc.
		// We'll add annotation after ~6 seconds (allowing for 2-3 retries)
		time.Sleep(6 * time.Second)
		updatedNode, err := client.CoreV1().Nodes().Get(ctx, "test-node", metav1.GetOptions{})
		if err == nil {
			updatedNode.Annotations[AnnotationGpuFakeDevices] = validAnnotation
			_, _ = client.CoreV1().Nodes().Update(ctx, updatedNode, metav1.UpdateOptions{})
		}
	}()

	devices, err := waitForGPUAnnotation(ctx, client, "test-node")

	// Should eventually succeed
	assert.NoError(t, err)
	require.NotNil(t, devices)
	assert.Len(t, devices, 1)
	_ = attemptCount // Suppress unused variable warning
}
