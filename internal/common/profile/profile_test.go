package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestLoad_ValidProfile(t *testing.T) {
	profileCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gpu-profile-h100",
			Namespace: "gpu-operator",
			Labels:    map[string]string{LabelGpuProfile: "true"},
		},
		Data: map[string]string{
			CmProfileKey: `
device_defaults:
  name: "NVIDIA H100 80GB HBM3"
  memory:
    total_bytes: 85899345920
  architecture: "Hopper"
system:
  driver_version: "550.163.01"
  cuda_version: "12.4"
devices:
  - {}
  - {}
  - {}
  - {}
  - {}
  - {}
  - {}
  - {}
`,
		},
	}

	client := fake.NewSimpleClientset(profileCM)
	data, err := Load(client, "gpu-operator", "h100")
	require.NoError(t, err)

	dd := data["device_defaults"].(map[string]interface{})
	assert.Equal(t, "NVIDIA H100 80GB HBM3", dd["name"])
	assert.Len(t, data["devices"].([]interface{}), 8)
}

func TestLoad_MissingProfile(t *testing.T) {
	client := fake.NewSimpleClientset()
	_, err := Load(client, "gpu-operator", "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestLoad_MissingKey(t *testing.T) {
	profileCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gpu-profile-bad",
			Namespace: "gpu-operator",
		},
		Data: map[string]string{},
	}

	client := fake.NewSimpleClientset(profileCM)
	_, err := Load(client, "gpu-operator", "bad")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing key")
}

func TestLoad_MalformedYAML(t *testing.T) {
	profileCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gpu-profile-broken",
			Namespace: "gpu-operator",
		},
		Data: map[string]string{
			CmProfileKey: `{{{not yaml`,
		},
	}

	client := fake.NewSimpleClientset(profileCM)
	_, err := Load(client, "gpu-operator", "broken")
	assert.Error(t, err)
}

func TestMerge_ScalarOverride(t *testing.T) {
	base := map[string]interface{}{
		"device_defaults": map[string]interface{}{
			"name": "NVIDIA H100 80GB HBM3",
			"memory": map[string]interface{}{
				"total_bytes": 85899345920,
			},
		},
	}
	overrides := map[string]interface{}{
		"device_defaults": map[string]interface{}{
			"name": "Custom H100",
		},
	}

	result := Merge(base, overrides)
	dd := result["device_defaults"].(map[string]interface{})
	assert.Equal(t, "Custom H100", dd["name"])
	mem := dd["memory"].(map[string]interface{})
	assert.Equal(t, 85899345920, mem["total_bytes"])
}

func TestMerge_ListReplacement(t *testing.T) {
	base := map[string]interface{}{
		"devices": []interface{}{
			map[string]interface{}{}, map[string]interface{}{},
			map[string]interface{}{}, map[string]interface{}{},
		},
	}
	overrides := map[string]interface{}{
		"devices": []interface{}{
			map[string]interface{}{}, map[string]interface{}{},
		},
	}

	result := Merge(base, overrides)
	assert.Len(t, result["devices"].([]interface{}), 2)
}

func TestMerge_NestedObjectMerge(t *testing.T) {
	base := map[string]interface{}{
		"system": map[string]interface{}{
			"driver_version": "550.163.01",
			"cuda_version":   "12.4",
		},
	}
	overrides := map[string]interface{}{
		"system": map[string]interface{}{
			"cuda_version": "12.6",
		},
	}

	result := Merge(base, overrides)
	sys := result["system"].(map[string]interface{})
	assert.Equal(t, "550.163.01", sys["driver_version"])
	assert.Equal(t, "12.6", sys["cuda_version"])
}

func TestMerge_EmptyOverrides(t *testing.T) {
	base := map[string]interface{}{
		"device_defaults": map[string]interface{}{
			"name": "NVIDIA H100 80GB HBM3",
		},
	}

	result := Merge(base, nil)
	dd := result["device_defaults"].(map[string]interface{})
	assert.Equal(t, "NVIDIA H100 80GB HBM3", dd["name"])

	result2 := Merge(base, map[string]interface{}{})
	dd2 := result2["device_defaults"].(map[string]interface{})
	assert.Equal(t, "NVIDIA H100 80GB HBM3", dd2["name"])
}

func TestMerge_AddNewKey(t *testing.T) {
	base := map[string]interface{}{
		"device_defaults": map[string]interface{}{
			"name": "NVIDIA H100 80GB HBM3",
		},
	}
	overrides := map[string]interface{}{
		"system": map[string]interface{}{
			"driver_version": "550.163.01",
		},
	}

	result := Merge(base, overrides)
	assert.Contains(t, result, "system")
	assert.Contains(t, result, "device_defaults")
}

func TestExtract_FullProfile(t *testing.T) {
	data := map[string]interface{}{
		"device_defaults": map[string]interface{}{
			"name": "NVIDIA H100 80GB HBM3",
			"memory": map[string]interface{}{
				"total_bytes": int64(85899345920),
			},
			"architecture": "Hopper",
		},
		"system": map[string]interface{}{
			"driver_version": "550.163.01",
			"cuda_version":   "12.4",
		},
		"devices": []interface{}{
			map[string]interface{}{}, map[string]interface{}{},
			map[string]interface{}{}, map[string]interface{}{},
			map[string]interface{}{}, map[string]interface{}{},
			map[string]interface{}{}, map[string]interface{}{},
		},
	}

	spec := Extract(data)
	assert.Equal(t, "NVIDIA H100 80GB HBM3", spec.GpuProduct)
	assert.Equal(t, 81920, spec.GpuMemory)
	assert.Equal(t, 8, spec.GpuCount)
	assert.Equal(t, "Hopper", spec.Architecture)
	assert.Equal(t, "550.163.01", spec.DriverVersion)
	assert.Equal(t, "12.4", spec.CudaVersion)
}

func TestExtract_MissingFields(t *testing.T) {
	data := map[string]interface{}{
		"device_defaults": map[string]interface{}{
			"name": "Minimal GPU",
		},
	}

	spec := Extract(data)
	assert.Equal(t, "Minimal GPU", spec.GpuProduct)
	assert.Equal(t, 0, spec.GpuMemory)
	assert.Equal(t, 0, spec.GpuCount)
	assert.Empty(t, spec.Architecture)
	assert.Empty(t, spec.DriverVersion)
}

func TestExtract_EmptyProfile(t *testing.T) {
	spec := Extract(map[string]interface{}{})
	assert.Empty(t, spec.GpuProduct)
	assert.Equal(t, 0, spec.GpuMemory)
	assert.Equal(t, 0, spec.GpuCount)
}

func TestExtract_NilProfile(t *testing.T) {
	spec := Extract(nil)
	assert.Empty(t, spec.GpuProduct)
	assert.Equal(t, 0, spec.GpuCount)
}

func TestExtract_DeviceCount(t *testing.T) {
	data := map[string]interface{}{
		"device_defaults": map[string]interface{}{
			"name": "GPU",
		},
		"device_count": 2,
	}
	spec := Extract(data)
	assert.Equal(t, 2, spec.GpuCount)
}

func TestExtract_DeviceCountOverridesDevicesList(t *testing.T) {
	data := map[string]interface{}{
		"device_defaults": map[string]interface{}{
			"name": "GPU",
		},
		"device_count": 2,
		"devices": []interface{}{
			map[string]interface{}{}, map[string]interface{}{},
			map[string]interface{}{}, map[string]interface{}{},
		},
	}
	spec := Extract(data)
	assert.Equal(t, 2, spec.GpuCount, "device_count should take priority over devices list")
}

func TestExtract_MemoryAsInt(t *testing.T) {
	data := map[string]interface{}{
		"device_defaults": map[string]interface{}{
			"name": "GPU",
			"memory": map[string]interface{}{
				"total_bytes": 17179869184,
			},
		},
	}
	spec := Extract(data)
	assert.Equal(t, 16384, spec.GpuMemory)
}
