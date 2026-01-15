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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"

	cdiparser "tags.cncf.io/container-device-interface/pkg/parser"
)

func TestNewComputeDomainCDIHandler(t *testing.T) {
	tmpDir := t.TempDir()
	cdiRoot := filepath.Join(tmpDir, "cdi")
	pluginRoot := filepath.Join(tmpDir, "plugin")

	err := os.MkdirAll(cdiRoot, 0750)
	require.NoError(t, err)
	err = os.MkdirAll(pluginRoot, 0750)
	require.NoError(t, err)

	config := &Config{
		flags: &Flags{
			cdiRoot:                     cdiRoot,
			kubeletPluginsDirectoryPath: pluginRoot,
		},
	}

	handler, err := NewComputeDomainCDIHandler(config)
	require.NoError(t, err)
	assert.NotNil(t, handler)
	assert.NotNil(t, handler.cache)
}

func TestComputeDomainCDIHandler_CreateCommonSpecFile(t *testing.T) {
	tmpDir := t.TempDir()
	cdiRoot := filepath.Join(tmpDir, "cdi")
	pluginRoot := filepath.Join(tmpDir, "plugin")

	err := os.MkdirAll(cdiRoot, 0750)
	require.NoError(t, err)
	err = os.MkdirAll(pluginRoot, 0750)
	require.NoError(t, err)

	config := &Config{
		flags: &Flags{
			cdiRoot:                     cdiRoot,
			kubeletPluginsDirectoryPath: pluginRoot,
		},
	}

	handler, err := NewComputeDomainCDIHandler(config)
	require.NoError(t, err)

	err = handler.CreateCommonSpecFile()
	require.NoError(t, err)

	// Verify spec file was created
	files, err := os.ReadDir(cdiRoot)
	require.NoError(t, err)
	assert.Greater(t, len(files), 0)
}

func TestComputeDomainCDIHandler_CreateClaimSpecFile(t *testing.T) {
	tmpDir := t.TempDir()
	cdiRoot := filepath.Join(tmpDir, "cdi")
	pluginRoot := filepath.Join(tmpDir, "plugin")

	err := os.MkdirAll(cdiRoot, 0750)
	require.NoError(t, err)
	err = os.MkdirAll(pluginRoot, 0750)
	require.NoError(t, err)

	config := &Config{
		flags: &Flags{
			cdiRoot:                     cdiRoot,
			kubeletPluginsDirectoryPath: pluginRoot,
		},
	}

	handler, err := NewComputeDomainCDIHandler(config)
	require.NoError(t, err)

	err = handler.CreateCommonSpecFile()
	require.NoError(t, err)

	// Create test prepared devices
	devices := ComputeDomainPreparedDevices{
		{
			Device: drapbv1.Device{
				DeviceName:   "domain",
				RequestNames: []string{"domain"},
				PoolName:     "default",
			},
		},
	}

	claimUID := "test-claim-uid"
	err = handler.CreateClaimSpecFile(claimUID, devices)
	require.NoError(t, err)

	// Verify spec file was created
	files, err := os.ReadDir(cdiRoot)
	require.NoError(t, err)
	assert.Greater(t, len(files), 1) // Common + claim spec
}

func TestComputeDomainCDIHandler_DeleteClaimSpecFile(t *testing.T) {
	tmpDir := t.TempDir()
	cdiRoot := filepath.Join(tmpDir, "cdi")
	pluginRoot := filepath.Join(tmpDir, "plugin")

	err := os.MkdirAll(cdiRoot, 0750)
	require.NoError(t, err)
	err = os.MkdirAll(pluginRoot, 0750)
	require.NoError(t, err)

	config := &Config{
		flags: &Flags{
			cdiRoot:                     cdiRoot,
			kubeletPluginsDirectoryPath: pluginRoot,
		},
	}

	handler, err := NewComputeDomainCDIHandler(config)
	require.NoError(t, err)

	claimUID := "test-claim-uid"

	// Delete non-existent spec should not error
	err = handler.DeleteClaimSpecFile(claimUID)
	assert.NoError(t, err)
}

func TestComputeDomainCDIHandler_GetClaimDevices(t *testing.T) {
	tmpDir := t.TempDir()
	cdiRoot := filepath.Join(tmpDir, "cdi")
	pluginRoot := filepath.Join(tmpDir, "plugin")

	err := os.MkdirAll(cdiRoot, 0750)
	require.NoError(t, err)
	err = os.MkdirAll(pluginRoot, 0750)
	require.NoError(t, err)

	config := &Config{
		flags: &Flags{
			cdiRoot:                     cdiRoot,
			kubeletPluginsDirectoryPath: pluginRoot,
		},
	}

	handler, err := NewComputeDomainCDIHandler(config)
	require.NoError(t, err)

	claimUID := "test-claim-uid"
	devices := []string{"domain"}

	cdiDevices := handler.GetClaimDevices(claimUID, devices)
	require.NotNil(t, cdiDevices)
	require.Len(t, cdiDevices, 1)

	expected := cdiparser.QualifiedName(
		computeDomainCDIVendor,
		computeDomainCDIClass,
		claimUID+"-"+devices[0],
	)
	assert.Equal(t, expected, cdiDevices[0])
}

func TestComputeDomainCDIHandler_CreateDomainCDIDevice(t *testing.T) {
	tmpDir := t.TempDir()
	cdiRoot := filepath.Join(tmpDir, "cdi")
	pluginRoot := filepath.Join(tmpDir, "plugin")

	err := os.MkdirAll(cdiRoot, 0750)
	require.NoError(t, err)
	err = os.MkdirAll(pluginRoot, 0750)
	require.NoError(t, err)

	config := &Config{
		flags: &Flags{
			cdiRoot:                     cdiRoot,
			kubeletPluginsDirectoryPath: pluginRoot,
		},
	}

	handler, err := NewComputeDomainCDIHandler(config)
	require.NoError(t, err)

	domainInfo := &DomainInfo{
		DomainID: "test-domain-id",
		Claims:   []string{"claim-uid"},
	}

	edits, err := handler.CreateDomainCDIDevice(domainInfo)
	require.NoError(t, err)
	require.NotNil(t, edits)
	require.NotNil(t, edits.DeviceNodes)
	require.Len(t, edits.DeviceNodes, 1)

	node := edits.DeviceNodes[0]
	assert.Contains(t, node.Path, "channel-0")
	assert.Equal(t, "c", node.Type)
}

func TestComputeDomainCDIConstants(t *testing.T) {
	assert.Equal(t, "k8s.compute-domain.nvidia.com", computeDomainCDIVendor)
	assert.Equal(t, "computedomain", computeDomainCDIClass)
	assert.Equal(t, "k8s.compute-domain.nvidia.com/computedomain", computeDomainCDIKind)
	assert.Equal(t, "common", computeDomainCDICommonDeviceName)
}
