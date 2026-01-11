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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"

	cdiparser "tags.cncf.io/container-device-interface/pkg/parser"

	"github.com/run-ai/fake-gpu-operator/pkg/compute-domain/consts"
)

func TestComputeDomainPreparedDevicesGetDevices(t *testing.T) {
	tests := map[string]struct {
		preparedDevices ComputeDomainPreparedDevices
		expected        []*drapbv1.Device
	}{
		"nil PreparedDevices": {
			preparedDevices: nil,
			expected:        nil,
		},
		"empty PreparedDevices": {
			preparedDevices: ComputeDomainPreparedDevices{},
			expected:        nil,
		},
		"several PreparedDevices": {
			preparedDevices: ComputeDomainPreparedDevices{
				{Device: drapbv1.Device{DeviceName: "domain-1"}},
				{Device: drapbv1.Device{DeviceName: "domain-2"}},
			},
			expected: []*drapbv1.Device{
				{DeviceName: "domain-1"},
				{DeviceName: "domain-2"},
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

func TestDomainInfo(t *testing.T) {
	domainInfo := &DomainInfo{
		DomainID: "test-domain-id",
		Claims:   []string{"claim-uid-1", "claim-uid-2"},
	}

	assert.Equal(t, "test-domain-id", domainInfo.DomainID)
	assert.Len(t, domainInfo.Claims, 2)
}

func TestNewComputeDomainState(t *testing.T) {
	// Create temporary directory for test
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
		},
		coreclient: fake.NewSimpleClientset(),
	}

	state, err := NewComputeDomainState(config)
	require.NoError(t, err)
	assert.NotNil(t, state)
	assert.NotNil(t, state.allocatable)
	assert.NotNil(t, state.domains)
	assert.NotNil(t, state.cdi)
	assert.NotNil(t, state.checkpointManager)
}

func TestComputeDomainState_Prepare(t *testing.T) {
	// This is a placeholder test - actual implementation will be added later
	// For now, test that the method exists and returns expected structure
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
		},
		coreclient: fake.NewSimpleClientset(),
	}

	state, err := NewComputeDomainState(config)
	require.NoError(t, err)

	claim := newTestComputeDomainClaim()

	preparedDevices, err := state.Prepare(claim)
	require.NoError(t, err)
	assert.NotNil(t, preparedDevices)
	assert.Len(t, preparedDevices, 1)
	assert.Equal(t, deviceNameForChannel(0), preparedDevices[0].DeviceName)
	require.Len(t, preparedDevices[0].CDIDeviceIDs, 1)
	require.NotNil(t, preparedDevices[0].ContainerEdits)
	require.NotNil(t, preparedDevices[0].ContainerEdits.ContainerEdits)
	require.NotNil(t, preparedDevices[0].ContainerEdits.ContainerEdits.DeviceNodes)
	require.Len(t, preparedDevices[0].ContainerEdits.ContainerEdits.DeviceNodes, 1)

	expectedCDIID := cdiparser.QualifiedName(
		computeDomainCDIVendor,
		computeDomainCDIClass,
		fmt.Sprintf("%s-%s", claim.UID, preparedDevices[0].DeviceName),
	)
	assert.Equal(t, expectedCDIID, preparedDevices[0].CDIDeviceIDs[0])
	assert.True(t, claimSpecExists(cdiRoot, string(claim.UID)))
}

func TestComputeDomainState_Unprepare(t *testing.T) {
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
		},
		coreclient: fake.NewSimpleClientset(),
	}

	state, err := NewComputeDomainState(config)
	require.NoError(t, err)

	claim := newTestComputeDomainClaim()

	_, err = state.Prepare(claim)
	require.NoError(t, err)
	require.True(t, claimSpecExists(cdiRoot, string(claim.UID)))

	err = state.Unprepare(string(claim.UID))
	assert.NoError(t, err)
	assert.False(t, claimSpecExists(cdiRoot, string(claim.UID)))
}

func newTestComputeDomainClaim() *resourceapi.ResourceClaim {
	return &resourceapi.ResourceClaim{
		ObjectMeta: metav1.ObjectMeta{
			UID:       types.UID("test-claim-uid"),
			Name:      "test-claim",
			Namespace: "default",
			Labels: map[string]string{
				"computedomain.resource.nvidia.com/computedomain": "test-domain",
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
	}
}

func claimSpecExists(cdiRoot, claimUID string) bool {
	entries, err := os.ReadDir(cdiRoot)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), claimUID) {
			return true
		}
	}
	return false
}
