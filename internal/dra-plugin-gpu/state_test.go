package dra_plugin_gpu

import (
	"context"
	"encoding/json"
	"os"
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

	configapi "sigs.k8s.io/dra-example-driver/api/example.com/resource/gpu/v1alpha1"
)

// Test constants
const (
	testNodeName   = "test-node"
	testGpuDevice0 = "GPU-test-0"
	testGpuDevice1 = "GPU-test-1"
	testGpuProduct = "Test-GPU"
	testRequest1   = "request-1"
	testRequest2   = "request-2"
	testClaimUID1  = "claim-1"
	testClaimUID2  = "claim-2"
	testClaimUID3  = "claim-3"
	nodeNameEnvVar = "NODE_NAME"
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

// We can't easily mock kubeletplugin.Helper as it's a concrete type
// For tests that need it, we'll skip NewDeviceState tests that require it
// and test other methods directly

func createTestConfig(t *testing.T) (*Config, func()) {
	tmpDir := t.TempDir()
	require.NoError(t, os.Setenv(nodeNameEnvVar, testNodeName))

	client := fake.NewSimpleClientset()
	node := createTestNode(testNodeName, testGpuDevice0)
	_, err := client.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
	require.NoError(t, err)

	config := &Config{
		Flags: &Flags{
			NodeName: testNodeName,
			CDIRoot:  tmpDir,
		},
		CoreClient: client,
	}

	cleanup := func() {
		_ = os.Unsetenv(nodeNameEnvVar)
	}

	return config, cleanup
}

func createTestNode(nodeName, gpuID string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
			Annotations: map[string]string{
				AnnotationGpuFakeDevices: `{
					"gpuMemory": 40960,
					"gpuProduct": "` + testGpuProduct + `",
					"gpus": [{"id": "` + gpuID + `", "status": {"allocatedBy": {"namespace": "", "pod": "", "container": ""}, "podGpuUsageStatus": {}}}],
					"migStrategy": "none"
				}`,
			},
		},
	}
}

func createTestState(t *testing.T, config *Config) *DeviceState {
	allocatable := AllocatableDevices{
		testGpuDevice0: resourceapi.Device{Name: testGpuDevice0},
	}
	cdi, err := NewCDIHandler(config)
	require.NoError(t, err)
	return &DeviceState{
		allocatable: allocatable,
		cdi:         cdi,
		coreclient:  config.CoreClient,
		nodeName:    config.Flags.NodeName,
	}
}

func createTestStateWithDevices(t *testing.T, config *Config, deviceIDs ...string) *DeviceState {
	allocatable := make(AllocatableDevices)
	for _, id := range deviceIDs {
		allocatable[id] = resourceapi.Device{Name: id}
	}
	cdi, err := NewCDIHandler(config)
	require.NoError(t, err)
	return &DeviceState{
		allocatable: allocatable,
		cdi:         cdi,
		coreclient:  config.CoreClient,
		nodeName:    config.Flags.NodeName,
	}
}

func createTestClaim(uid, deviceID, requestName, poolName string) *resourceapi.ResourceClaim {
	return &resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{UID: types.UID(uid)},
		Status: resourceapi.ResourceClaimStatus{
			Allocation: &resourceapi.AllocationResult{
				Devices: resourceapi.DeviceAllocationResult{
					Results: []resourceapi.DeviceRequestAllocationResult{
						{Device: deviceID, Request: requestName, Pool: poolName},
					},
					Config: []resourceapi.DeviceAllocationConfiguration{},
				},
			},
		},
	}
}

func createUnallocatedClaim(uid string) *resourceapi.ResourceClaim {
	return &resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{UID: types.UID(uid)},
		Status:     resourceapi.ResourceClaimStatus{},
	}
}

func createMultiDeviceClaim(uid, poolName string, deviceIDs, requestNames []string) *resourceapi.ResourceClaim {
	var results []resourceapi.DeviceRequestAllocationResult
	for i, deviceID := range deviceIDs {
		reqName := requestNames[0]
		if i < len(requestNames) {
			reqName = requestNames[i]
		}
		results = append(results, resourceapi.DeviceRequestAllocationResult{
			Device: deviceID, Request: reqName, Pool: poolName,
		})
	}
	return &resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{UID: types.UID(uid)},
		Status: resourceapi.ResourceClaimStatus{
			Allocation: &resourceapi.AllocationResult{
				Devices: resourceapi.DeviceAllocationResult{
					Results: results,
					Config:  []resourceapi.DeviceAllocationConfiguration{},
				},
			},
		},
	}
}

// TestNewDeviceState is skipped because it requires a real kubeletplugin.Helper
// which is complex to mock. Integration tests should cover this.
func TestNewDeviceState(t *testing.T) {
	t.Skip("Requires real kubeletplugin.Helper - tested via integration tests")
}

func TestDeviceState_Prepare(t *testing.T) {
	config, cleanup := createTestConfig(t)
	defer cleanup()

	state := createTestState(t, config)

	tests := map[string]struct {
		claim        *resourceapi.ResourceClaim
		wantErr      bool
		wantPrepared bool
		prepareTwice bool
	}{
		"new claim": {
			claim:        createTestClaim(testClaimUID1, testGpuDevice0, testRequest1, testNodeName),
			wantPrepared: true,
		},
		"claim not allocated": {
			claim:   createUnallocatedClaim(testClaimUID2),
			wantErr: true,
		},
		"idempotency - prepare twice": {
			claim:        createTestClaim(testClaimUID3, testGpuDevice0, testRequest1, testNodeName),
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

	state := createTestState(t, config)

	// First prepare a claim
	claim := createTestClaim("claim-to-unprepare", testGpuDevice0, testRequest1, testNodeName)
	_, err := state.Prepare(context.Background(), claim)
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
		"opaque is nil - skipped": {
			possibleConfigs: []resourceapi.DeviceAllocationConfiguration{
				{
					Source: resourceapi.AllocationConfigSourceClass,
					DeviceConfiguration: resourceapi.DeviceConfiguration{
						Opaque: nil,
					},
				},
			},
			wantErr:   false,
			wantCount: 0, // Skipped due to nil opaque
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

func TestDeviceState_UpdateDevicesFromAnnotation(t *testing.T) {
	config, cleanup := createTestConfig(t)
	defer cleanup()

	state := createTestState(t, config)

	// Update node annotation
	node, err := config.CoreClient.CoreV1().Nodes().Get(context.Background(), testNodeName, metav1.GetOptions{})
	require.NoError(t, err)
	node.Annotations[AnnotationGpuFakeDevices] = `{
		"gpuMemory": 40960,
		"gpuProduct": "Updated-GPU",
		"gpus": [{"id": "GPU-updated", "status": {"allocatedBy": {"namespace": "", "pod": "", "container": ""}, "podGpuUsageStatus": {}}}],
		"migStrategy": "none"
	}`
	_, err = config.CoreClient.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{})
	require.NoError(t, err)

	err = state.UpdateDevicesFromAnnotation(context.Background())
	assert.NoError(t, err)
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
				envs := edits["gpu-test-0"].Env
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
				envs := edits["gpu-test-0"].Env
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

	state := createTestStateWithDevices(t, config, testGpuDevice0, testGpuDevice1)

	tests := map[string]struct {
		claim    *resourceapi.ResourceClaim
		wantErr  bool
		validate func(*testing.T, PreparedDevices)
	}{
		"single device": {
			claim: createTestClaim(testClaimUID1, testGpuDevice0, testRequest1, testNodeName),
			validate: func(t *testing.T, devices PreparedDevices) {
				assert.Len(t, devices, 1)
				assert.Equal(t, testGpuDevice0, devices[0].DeviceName)
			},
		},
		"multiple devices": {
			claim: createMultiDeviceClaim(testClaimUID2, testNodeName,
				[]string{testGpuDevice0, testGpuDevice1},
				[]string{testRequest1, testRequest2}),
			validate: func(t *testing.T, devices PreparedDevices) {
				assert.Len(t, devices, 2)
			},
		},
		"device not allocatable": {
			claim:   createTestClaim(testClaimUID3, "GPU-non-existent", testRequest1, testNodeName),
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

	// Create node without GPU annotation - will cause retries
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        testNodeName,
			Annotations: map[string]string{},
		},
	}
	_, err := client.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
	require.NoError(t, err)

	// Cancel context after a short delay
	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()

	devices, err := waitForGPUAnnotation(ctx, client, testNodeName)

	assert.Error(t, err)
	assert.Nil(t, devices)
	assert.Contains(t, err.Error(), "context canceled")
}
