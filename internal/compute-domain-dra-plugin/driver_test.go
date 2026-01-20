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

package computedomaindraplugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/klog/v2"

	"github.com/run-ai/fake-gpu-operator/pkg/compute-domain/consts"
)

func TestComputeDomainDriver_PrepareResourceClaims(t *testing.T) {
	tmpDir := t.TempDir()
	cdiRoot := filepath.Join(tmpDir, "cdi")
	pluginPath := filepath.Join(tmpDir, "plugin")

	err := os.MkdirAll(cdiRoot, 0750)
	require.NoError(t, err)
	err = os.MkdirAll(pluginPath, 0750)
	require.NoError(t, err)

	config := &Config{
		flags: &Flags{
			cdiRoot:                     cdiRoot,
			nodeName:                    "test-node",
			kubeletPluginsDirectoryPath: pluginPath,
			healthcheckPort:             -1, // Disable healthcheck for test
		},
		coreclient: fake.NewSimpleClientset(),
	}

	ctx := context.Background()
	driver, err := NewComputeDomainDriver(ctx, config)
	require.NoError(t, err)
	assert.NotNil(t, driver)

	claims := []*resourceapi.ResourceClaim{
		{
			ObjectMeta: metav1.ObjectMeta{
				UID:       types.UID("test-claim-uid"),
				Name:      "test-claim",
				Namespace: "default",
				Labels: map[string]string{
					consts.ComputeDomainTemplateLabel: "test-compute-domain",
				},
			},
			Spec: resourceapi.ResourceClaimSpec{
				Devices: resourceapi.DeviceClaim{
					Config: []resourceapi.DeviceClaimConfiguration{
						{
							DeviceConfiguration: resourceapi.DeviceConfiguration{
								Opaque: &resourceapi.OpaqueDeviceConfiguration{
									Driver: consts.ComputeDomainDriverName,
									Parameters: runtime.RawExtension{
										Raw: []byte(`{
											"allocationMode": "Single",
											"apiVersion": "resource.nvidia.com/v1beta1",
											"domainID": "test-compute-domain",
											"kind": "ComputeDomainChannelConfig"
										}`),
									},
								},
							},
						},
					},
					Requests: []resourceapi.DeviceRequest{
						{
							Name: "channel",
							Exactly: &resourceapi.ExactDeviceRequest{
								AllocationMode:  resourceapi.DeviceAllocationModeExactCount,
								Count:           1,
								DeviceClassName: consts.ComputeDomainWorkloadDeviceClass,
							},
						},
					},
				},
			},
		},
	}

	results, err := driver.PrepareResourceClaims(ctx, claims)
	require.NoError(t, err)
	assert.NotNil(t, results)
	assert.Len(t, results, 1)

	result, exists := results[types.UID("test-claim-uid")]
	assert.True(t, exists)
	assert.NotNil(t, result)
	assert.NoError(t, result.Err)
	assert.NotNil(t, result.Devices)
}

func TestComputeDomainDriver_UnprepareResourceClaims(t *testing.T) {
	tmpDir := t.TempDir()
	// Use shorter paths to avoid Unix socket path length limits
	cdiRoot := filepath.Join(tmpDir, "cdi")
	pluginPath := filepath.Join(tmpDir, "p")

	err := os.MkdirAll(cdiRoot, 0750)
	require.NoError(t, err)
	err = os.MkdirAll(pluginPath, 0750)
	require.NoError(t, err)

	config := &Config{
		flags: &Flags{
			cdiRoot:                     cdiRoot,
			nodeName:                    "test-node",
			kubeletPluginsDirectoryPath: pluginPath,
			healthcheckPort:             -1,
		},
		coreclient: fake.NewSimpleClientset(),
	}

	ctx := context.Background()
	driver, err := NewComputeDomainDriver(ctx, config)
	require.NoError(t, err)

	claims := []kubeletplugin.NamespacedObject{
		{
			UID: types.UID("test-claim-uid"),
		},
	}

	results, err := driver.UnprepareResourceClaims(ctx, claims)
	require.NoError(t, err)
	assert.NotNil(t, results)
	assert.Len(t, results, 1)

	err, exists := results[types.UID("test-claim-uid")]
	assert.True(t, exists)
	assert.NoError(t, err)
}

func TestComputeDomainDriver_Shutdown(t *testing.T) {
	tmpDir := t.TempDir()
	cdiRoot := filepath.Join(tmpDir, "cdi")
	pluginPath := filepath.Join(tmpDir, "plugin")

	err := os.MkdirAll(cdiRoot, 0750)
	require.NoError(t, err)
	err = os.MkdirAll(pluginPath, 0750)
	require.NoError(t, err)

	config := &Config{
		flags: &Flags{
			cdiRoot:                     cdiRoot,
			nodeName:                    "test-node",
			kubeletPluginsDirectoryPath: pluginPath,
			healthcheckPort:             -1,
		},
		coreclient: fake.NewSimpleClientset(),
	}

	ctx := context.Background()
	driver, err := NewComputeDomainDriver(ctx, config)
	require.NoError(t, err)

	logger := klog.FromContext(ctx)
	err = driver.Shutdown(logger)
	assert.NoError(t, err)
}

func TestComputeDomainDriver_HandleError(t *testing.T) {
	tmpDir := t.TempDir()
	cdiRoot := filepath.Join(tmpDir, "cdi")
	pluginPath := filepath.Join(tmpDir, "plugin")

	err := os.MkdirAll(cdiRoot, 0750)
	require.NoError(t, err)
	err = os.MkdirAll(pluginPath, 0750)
	require.NoError(t, err)

	cancelCalled := false
	cancelCtx := func(error) {
		cancelCalled = true
	}

	config := &Config{
		flags: &Flags{
			cdiRoot:                     cdiRoot,
			nodeName:                    "test-node",
			kubeletPluginsDirectoryPath: pluginPath,
			healthcheckPort:             -1,
		},
		coreclient:    fake.NewSimpleClientset(),
		cancelMainCtx: cancelCtx,
	}

	ctx := context.Background()
	driver, err := NewComputeDomainDriver(ctx, config)
	require.NoError(t, err)

	// Test recoverable error
	recoverableErr := kubeletplugin.ErrRecoverable
	driver.HandleError(ctx, recoverableErr, "test error")
	assert.False(t, cancelCalled, "recoverable error should not cancel context")

	// Test fatal error
	fatalErr := assert.AnError
	driver.HandleError(ctx, fatalErr, "fatal error")
	assert.True(t, cancelCalled, "fatal error should cancel context")
}
