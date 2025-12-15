package dra_plugin_gpu

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"

	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdiparser "tags.cncf.io/container-device-interface/pkg/parser"
	cdispec "tags.cncf.io/container-device-interface/specs-go"
)

// Test constants for CDI tests
const (
	cdiTestNodeName = "test-node"
	cdiTestGpu0     = "gpu-0"
	cdiTestGpu1     = "gpu-1"
	cdiTestClaim1   = "claim-123"
	cdiTestClaim2   = "claim-456"
	cdiTestClaim3   = "claim-789"
)

func TestNewCDIHandler(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Flags: &Flags{
			CDIRoot: tmpDir,
		},
	}

	handler, err := NewCDIHandler(config)
	require.NoError(t, err)
	require.NotNil(t, handler)
	require.NotNil(t, handler.cache)
}

func TestCDIHandler_CreateCommonSpecFile(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.Setenv("NODE_NAME", cdiTestNodeName))
	defer func() {
		_ = os.Unsetenv("NODE_NAME")
	}()

	config := &Config{
		Flags: &Flags{
			CDIRoot:  tmpDir,
			NodeName: cdiTestNodeName,
		},
	}

	handler, err := NewCDIHandler(config)
	require.NoError(t, err)

	err = handler.CreateCommonSpecFile()
	assert.NoError(t, err)

	// Verify spec file was created
	initialFiles, err := filepath.Glob(filepath.Join(tmpDir, "*"))
	require.NoError(t, err)
	assert.NotEmpty(t, initialFiles)

	// Verify the spec contains nvidia-smi mount by reading spec files
	// CDI creates spec files in vendor/class subdirectories
	var specFiles []string
	err = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			specFiles = append(specFiles, path)
		}
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, specFiles, "should have at least one spec file")

	// Read and parse spec files to find the common spec
	var foundMount bool
	var foundNodeName bool
	for _, specFile := range specFiles {
		specData, err := os.ReadFile(specFile)
		if err != nil {
			continue
		}

		var spec cdispec.Spec
		// Try JSON first, then YAML
		err = json.Unmarshal(specData, &spec)
		if err != nil {
			// Try YAML parsing
			err = yaml.Unmarshal(specData, &spec)
			if err != nil {
				continue // Skip files that can't be parsed
			}
		}

		// Find the common device
		for _, device := range spec.Devices {
			if device.Name == cdiCommonDeviceName {
				// Verify NODE_NAME environment variable exists
				if device.ContainerEdits.Env != nil {
					for _, env := range device.ContainerEdits.Env {
						if strings.HasPrefix(env, "NODE_NAME=") {
							foundNodeName = true
							assert.Equal(t, "NODE_NAME=test-node", env, "NODE_NAME should be set to test-node")
							break
						}
					}
				}
				// Verify nvidia-smi mount exists
				if device.ContainerEdits.Mounts != nil {
					for _, mount := range device.ContainerEdits.Mounts {
						if mount != nil && mount.HostPath == "/var/lib/runai/bin/nvidia-smi" && mount.ContainerPath == "/bin/nvidia-smi" {
							foundMount = true
							// Verify mount is read-only via Options
							hasRO := false
							for _, opt := range mount.Options {
								if opt == "ro" {
									hasRO = true
									break
								}
							}
							assert.True(t, hasRO, "nvidia-smi mount should have 'ro' option")
							break
						}
					}
				}
				break
			}
		}
		if foundMount && foundNodeName {
			break
		}
	}
	assert.True(t, foundMount, "nvidia-smi mount should be present in common spec")
	assert.True(t, foundNodeName, "NODE_NAME environment variable should be present in common spec")
}

func TestCDIHandler_CreateClaimSpecFile(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Flags: &Flags{
			CDIRoot: tmpDir,
		},
	}

	handler, err := NewCDIHandler(config)
	require.NoError(t, err)

	tests := map[string]struct {
		claimUID string
		devices  PreparedDevices
		wantErr  bool
	}{
		"single device": {
			claimUID: cdiTestClaim1,
			devices: PreparedDevices{{
				Device: drapbv1.Device{DeviceName: cdiTestGpu0},
				ContainerEdits: &cdiapi.ContainerEdits{
					ContainerEdits: &cdispec.ContainerEdits{Env: []string{"GPU_DEVICE_gpu_0=gpu-0"}},
				},
			}},
		},
		"multiple devices": {
			claimUID: cdiTestClaim2,
			devices: PreparedDevices{
				{
					Device: drapbv1.Device{DeviceName: cdiTestGpu0},
					ContainerEdits: &cdiapi.ContainerEdits{
						ContainerEdits: &cdispec.ContainerEdits{Env: []string{"GPU_DEVICE_gpu_0=gpu-0"}},
					},
				},
				{
					Device: drapbv1.Device{DeviceName: cdiTestGpu1},
					ContainerEdits: &cdiapi.ContainerEdits{
						ContainerEdits: &cdispec.ContainerEdits{Env: []string{"GPU_DEVICE_gpu_1=gpu-1"}},
					},
				},
			},
		},
		"device with container edits": {
			claimUID: cdiTestClaim3,
			devices: PreparedDevices{{
				Device: drapbv1.Device{DeviceName: cdiTestGpu0},
				ContainerEdits: &cdiapi.ContainerEdits{
					ContainerEdits: &cdispec.ContainerEdits{Env: []string{"TEST_VAR=value"}},
				},
			}},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := handler.CreateClaimSpecFile(test.claimUID, test.devices)
			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCDIHandler_DeleteClaimSpecFile(t *testing.T) {
	tmpDir := t.TempDir()

	config := &Config{
		Flags: &Flags{
			CDIRoot: tmpDir,
		},
	}

	handler, err := NewCDIHandler(config)
	require.NoError(t, err)

	// Create a spec file first
	devices := PreparedDevices{{
		Device: drapbv1.Device{DeviceName: cdiTestGpu0},
		ContainerEdits: &cdiapi.ContainerEdits{
			ContainerEdits: &cdispec.ContainerEdits{Env: []string{"GPU_DEVICE_gpu_0=gpu-0"}},
		},
	}}
	claimUID := "claim-to-delete"
	err = handler.CreateClaimSpecFile(claimUID, devices)
	require.NoError(t, err)

	// Now delete it
	err = handler.DeleteClaimSpecFile(claimUID)
	assert.NoError(t, err)
}

func TestCDIHandler_GetClaimDevices(t *testing.T) {
	handler := &CDIHandler{}

	tests := map[string]struct {
		claimUID string
		devices  []string
		expected []string
	}{
		"single device": {
			claimUID: "claim-1",
			devices:  []string{"gpu-0"},
			expected: []string{
				cdiparser.QualifiedName(cdiVendor, cdiClass, cdiCommonDeviceName),
				cdiparser.QualifiedName(cdiVendor, cdiClass, "claim-1-gpu-0"),
			},
		},
		"multiple devices": {
			claimUID: "claim-2",
			devices:  []string{"gpu-0", "gpu-1"},
			expected: []string{
				cdiparser.QualifiedName(cdiVendor, cdiClass, cdiCommonDeviceName),
				cdiparser.QualifiedName(cdiVendor, cdiClass, "claim-2-gpu-0"),
				cdiparser.QualifiedName(cdiVendor, cdiClass, "claim-2-gpu-1"),
			},
		},
		"empty devices": {
			claimUID: "claim-3",
			devices:  []string{},
			expected: []string{
				cdiparser.QualifiedName(cdiVendor, cdiClass, cdiCommonDeviceName),
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			result := handler.GetClaimDevices(test.claimUID, test.devices)
			assert.Equal(t, test.expected, result)
		})
	}
}

// Note: CDI error scenarios require mocking the CDI library which is complex.
// Error handling is covered by integration tests.
