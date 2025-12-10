package dra_plugin_gpu

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager"
)

func TestDriver_Shutdown(t *testing.T) {
	tests := map[string]struct {
		healthcheck *healthcheck
		wantErr     bool
	}{
		"with healthcheck": {
			healthcheck: &healthcheck{},
			wantErr:     false,
		},
		"without healthcheck": {
			healthcheck: nil,
			wantErr:     false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			d := &Driver{
				healthcheck: test.healthcheck,
				helper:      nil, // Helper not needed for Shutdown test
			}

			logger := klog.Background()
			err := d.Shutdown(logger)
			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDriver_PrepareResourceClaims(t *testing.T) {
	config, cleanup := createTestConfigForDriver(t)
	defer cleanup()

	state, err := createTestDeviceState(t, config)
	require.NoError(t, err)

	d := &Driver{
		state: state,
	}

	tests := map[string]struct {
		claims    []*resourceapi.ResourceClaim
		wantErr   bool
		wantCount int
	}{
		"single claim": {
			claims: []*resourceapi.ResourceClaim{
				createTestClaim("claim-1"),
			},
			wantErr:   false,
			wantCount: 1,
		},
		"multiple claims": {
			claims: []*resourceapi.ResourceClaim{
				createTestClaim("claim-1"),
				createTestClaim("claim-2"),
			},
			wantErr:   false,
			wantCount: 2,
		},
		"empty claims": {
			claims:    []*resourceapi.ResourceClaim{},
			wantErr:   false,
			wantCount: 0,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			results, err := d.PrepareResourceClaims(ctx, test.claims)
			if test.wantErr {
				assert.Error(t, err)
				assert.Nil(t, results)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, results)
				assert.Len(t, results, test.wantCount)
			}
		})
	}
}

func TestDriver_PrepareResourceClaim(t *testing.T) {
	config, cleanup := createTestConfigForDriver(t)
	defer cleanup()

	state, err := createTestDeviceState(t, config)
	require.NoError(t, err)

	d := &Driver{
		state: state,
	}

	tests := map[string]struct {
		claim       *resourceapi.ResourceClaim
		wantErr     bool
		wantDevices bool
	}{
		"successful preparation": {
			claim:       createTestClaim("claim-1"),
			wantErr:     false,
			wantDevices: true,
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
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			result := d.prepareResourceClaim(ctx, test.claim)
			if test.wantErr {
				assert.NotNil(t, result.Err)
				assert.Nil(t, result.Devices)
			} else {
				assert.Nil(t, result.Err)
				if test.wantDevices {
					assert.NotNil(t, result.Devices)
				}
			}
		})
	}
}

func TestDriver_UnprepareResourceClaims(t *testing.T) {
	config, cleanup := createTestConfigForDriver(t)
	defer cleanup()

	state, err := createTestDeviceState(t, config)
	require.NoError(t, err)

	d := &Driver{
		state: state,
	}

	// Prepare a claim first
	claim := createTestClaim("claim-to-unprepare")
			_, err = state.Prepare(context.Background(), claim)
	require.NoError(t, err)

	tests := map[string]struct {
		claims    []kubeletplugin.NamespacedObject
		wantErr   bool
		wantCount int
	}{
		"single claim": {
			claims: []kubeletplugin.NamespacedObject{
				{
					UID: types.UID("claim-to-unprepare"),
				},
			},
			wantErr:   false,
			wantCount: 1,
		},
		"multiple claims": {
			claims: []kubeletplugin.NamespacedObject{
				{
					UID: types.UID("claim-1"),
				},
				{
					UID: types.UID("claim-2"),
				},
			},
			wantErr:   false,
			wantCount: 2,
		},
		"empty claims": {
			claims:    []kubeletplugin.NamespacedObject{},
			wantErr:   false,
			wantCount: 0,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			results, err := d.UnprepareResourceClaims(ctx, test.claims)
			if test.wantErr {
				assert.Error(t, err)
				assert.Nil(t, results)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, results)
				assert.Len(t, results, test.wantCount)
			}
		})
	}
}

func TestDriver_UnprepareResourceClaim(t *testing.T) {
	config, cleanup := createTestConfigForDriver(t)
	defer cleanup()

	state, err := createTestDeviceState(t, config)
	require.NoError(t, err)

	d := &Driver{
		state: state,
	}

	// Prepare a claim first
	claim := createTestClaim("claim-to-unprepare")
			_, err = state.Prepare(context.Background(), claim)
	require.NoError(t, err)

	tests := map[string]struct {
		claim   kubeletplugin.NamespacedObject
		wantErr bool
	}{
		"successful unpreparation": {
			claim: kubeletplugin.NamespacedObject{
				UID: types.UID("claim-to-unprepare"),
			},
			wantErr: false,
		},
		"non-existent claim": {
			claim: kubeletplugin.NamespacedObject{
				UID: types.UID("non-existent"),
			},
			wantErr: false, // Should be idempotent
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			err := d.unprepareResourceClaim(ctx, test.claim)
			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDriver_HandleError(t *testing.T) {
	tests := map[string]struct {
		err       error
		cancelCtx func(error)
		called    bool
	}{
		"recoverable error": {
			err: kubeletplugin.ErrRecoverable,
			cancelCtx: func(err error) {
				// Should not be called
			},
			called: false,
		},
		"non-recoverable error": {
			err: errors.New("fatal error"),
			cancelCtx: func(err error) {
				// Should be called
			},
			called: true,
		},
		"nil cancelCtx": {
			err:       errors.New("fatal error"),
			cancelCtx: nil,
			called:    false, // Should not panic
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			called := false
			var actualCancelCtx func(error)
			if test.cancelCtx != nil {
				actualCancelCtx = func(err error) {
					called = true
					test.cancelCtx(err)
				}
			}

			d := &Driver{
				cancelCtx: actualCancelCtx,
			}

			ctx := context.Background()
			d.HandleError(ctx, test.err, "test error")

			assert.Equal(t, test.called, called)
		})
	}
}

// Helper functions for driver tests

func createTestConfigForDriver(t *testing.T) (*Config, func()) {
	tmpDir := t.TempDir()
	require.NoError(t, os.Setenv("NODE_NAME", "test-node"))

	client := fake.NewSimpleClientset()
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
			Annotations: map[string]string{
				AnnotationGpuFakeDevices: `{
					"version": "v1",
					"gpus": [
						{
							"uuid": "GPU-test-0",
							"minor": 0,
							"productName": "Test-GPU",
							"memoryBytes": 42949672960
						}
					]
				}`,
			},
		},
	}
	_, err := client.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
	require.NoError(t, err)

	config := &Config{
		Flags: &Flags{
			NodeName:                      "test-node",
			CDIRoot:                       tmpDir,
			KubeletPluginsDirectoryPath:   tmpDir,
			KubeletRegistrarDirectoryPath: tmpDir,
		},
		CoreClient: client,
	}

	cleanup := func() {
		_ = os.Unsetenv("NODE_NAME")
	}

	return config, cleanup
}

func createTestDeviceState(t *testing.T, config *Config) (*DeviceState, error) {
	// Use the same checkpoint directory as NewDeviceState would use
	checkpointDir := config.DriverPluginPath()
	require.NoError(t, os.MkdirAll(checkpointDir, 0755))

	checkpointManager, err := checkpointmanager.NewCheckpointManager(checkpointDir)
	require.NoError(t, err)

	// Initialize checkpoint (normally done in NewDeviceState)
	checkpoint := newCheckpoint()
	err = checkpointManager.CreateCheckpoint(DriverPluginCheckpointFile, checkpoint)
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
		coreclient:        config.CoreClient,
		nodeName:          config.Flags.NodeName,
	}

	return state, nil
}

func createTestClaim(uid string) *resourceapi.ResourceClaim {
	return &resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			UID: types.UID(uid),
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
}
