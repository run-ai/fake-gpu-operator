package topology

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNormalizeOldFormatToClusterConfig(t *testing.T) {
	old := &ClusterTopology{
		NodePoolLabelKey: "run.ai/simulated-gpu-node-pool",
		MigStrategy:      "mixed",
		NodePools: map[string]NodePoolTopology{
			"default": {
				GpuCount:   2,
				GpuMemory:  11441,
				GpuProduct: "Tesla-K80",
			},
		},
	}

	config := normalizeOldToClusterConfig(old)

	assert.Equal(t, "run.ai/simulated-gpu-node-pool", config.NodePoolLabelKey)
	assert.Equal(t, "mixed", config.MigStrategy)
	assert.Nil(t, config.GpuOperator)

	require.Contains(t, config.NodePools, "default")
	pool := config.NodePools["default"]

	assert.Equal(t, "fake", pool.Gpu.Backend)
	assert.Empty(t, pool.Gpu.Profile)

	// gpuProduct → overrides.device_defaults.name
	deviceDefaults := pool.Gpu.Overrides["device_defaults"].(map[string]interface{})
	assert.Equal(t, "Tesla-K80", deviceDefaults["name"])

	// gpuMemory (MiB) → overrides.device_defaults.memory.total_bytes (bytes)
	memory := deviceDefaults["memory"].(map[string]interface{})
	assert.Equal(t, int64(11441)*1024*1024, memory["total_bytes"])

	// gpuCount → overrides.devices (list of N empty items)
	devices := pool.Gpu.Overrides["devices"].([]interface{})
	assert.Len(t, devices, 2)
}

func TestNormalizeOldFormatWithOtherDevices(t *testing.T) {
	old := &ClusterTopology{
		NodePoolLabelKey: "run.ai/simulated-gpu-node-pool",
		MigStrategy:      "none",
		NodePools: map[string]NodePoolTopology{
			"rdma-pool": {
				GpuCount:   4,
				GpuMemory:  40960,
				GpuProduct: "NVIDIA-A100-SXM4-40GB",
				OtherDevices: []GenericDevice{
					{Name: "rdma/rdma_shared_device_a", Count: 1},
					{Name: "vpc.amazonaws.com/efa", Count: 4},
				},
			},
		},
	}

	config := normalizeOldToClusterConfig(old)
	pool := config.NodePools["rdma-pool"]

	// otherDevices → resources ([]map[string]int)
	require.Len(t, pool.Resources, 2)
	assert.Equal(t, map[string]int{"rdma/rdma_shared_device_a": 1}, pool.Resources[0])
	assert.Equal(t, map[string]int{"vpc.amazonaws.com/efa": 4}, pool.Resources[1])
}

func TestNormalizeOldFormatZeroGpuCount(t *testing.T) {
	old := &ClusterTopology{
		NodePoolLabelKey: "run.ai/simulated-gpu-node-pool",
		MigStrategy:      "none",
		NodePools: map[string]NodePoolTopology{
			"empty": {
				GpuCount:   0,
				GpuMemory:  0,
				GpuProduct: "",
			},
		},
	}

	config := normalizeOldToClusterConfig(old)
	pool := config.NodePools["empty"]

	assert.Equal(t, "fake", pool.Gpu.Backend)
	devices := pool.Gpu.Overrides["devices"].([]interface{})
	assert.Len(t, devices, 0)
}

func TestNormalizeOldFormatEmptyNodePools(t *testing.T) {
	old := &ClusterTopology{
		NodePoolLabelKey: "run.ai/simulated-gpu-node-pool",
		MigStrategy:      "none",
		NodePools:        map[string]NodePoolTopology{},
	}

	config := normalizeOldToClusterConfig(old)
	assert.Empty(t, config.NodePools)
}

func TestNormalizeOldFormatNilNodePools(t *testing.T) {
	old := &ClusterTopology{
		NodePoolLabelKey: "run.ai/simulated-gpu-node-pool",
		MigStrategy:      "none",
		NodePools:        nil,
	}

	config := normalizeOldToClusterConfig(old)
	assert.Empty(t, config.NodePools)
}

func TestDetectFormatOld(t *testing.T) {
	yamlData := `
nodePoolLabelKey: run.ai/simulated-gpu-node-pool
migStrategy: mixed
nodePools:
  default:
    gpuProduct: Tesla-K80
    gpuCount: 2
    gpuMemory: 11441
`
	config, err := ParseAndNormalizeTopology([]byte(yamlData))
	require.NoError(t, err)
	assert.Equal(t, "run.ai/simulated-gpu-node-pool", config.NodePoolLabelKey)
	assert.Equal(t, "fake", config.NodePools["default"].Gpu.Backend)
}

func TestDetectFormatNew(t *testing.T) {
	yamlData := `
nodePoolLabelKey: run.ai/simulated-gpu-node-pool
migStrategy: mixed
nodePools:
  pool-a:
    gpu:
      backend: fake
      profile: h100
      overrides:
        device_defaults:
          name: "Custom H100"
    resources:
      - rdma/rdma_shared_device_a: 1
`
	config, err := ParseAndNormalizeTopology([]byte(yamlData))
	require.NoError(t, err)
	assert.Equal(t, "run.ai/simulated-gpu-node-pool", config.NodePoolLabelKey)

	pool := config.NodePools["pool-a"]
	assert.Equal(t, "fake", pool.Gpu.Backend)
	assert.Equal(t, "h100", pool.Gpu.Profile)
	assert.Equal(t, "Custom H100", pool.Gpu.Overrides["device_defaults"].(map[string]interface{})["name"])
}

func TestDetectFormatNewWithGpuOperator(t *testing.T) {
	yamlData := `
nodePoolLabelKey: run.ai/simulated-gpu-node-pool
migStrategy: mixed
gpuOperator:
  version: v24.9.0
  values:
    dcgmExporter:
      enabled: true
nodePools:
  pool-b:
    gpu:
      backend: mock
      profile: a100
`
	config, err := ParseAndNormalizeTopology([]byte(yamlData))
	require.NoError(t, err)
	require.NotNil(t, config.GpuOperator)
	assert.Equal(t, "v24.9.0", config.GpuOperator.Version)
	assert.Equal(t, "mock", config.NodePools["pool-b"].Gpu.Backend)
}

func TestParseAndNormalizeInvalidYAML(t *testing.T) {
	_, err := ParseAndNormalizeTopology([]byte(`{{{not yaml`))
	assert.Error(t, err)
}

func TestParseAndNormalizeEmptyInput(t *testing.T) {
	_, err := ParseAndNormalizeTopology([]byte(""))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")

	_, err = ParseAndNormalizeTopology([]byte("   \n  "))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestFromClusterConfigCM_OldFormat(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "topology", Namespace: "gpu-operator"},
		Data: map[string]string{
			CmTopologyKey: `
nodePoolLabelKey: run.ai/simulated-gpu-node-pool
migStrategy: mixed
nodePools:
  default:
    gpuProduct: Tesla-K80
    gpuCount: 2
    gpuMemory: 11441
`,
		},
	}

	config, err := FromClusterConfigCM(cm)
	require.NoError(t, err)
	assert.Equal(t, "mixed", config.MigStrategy)
	assert.Equal(t, "fake", config.NodePools["default"].Gpu.Backend)
}

func TestFromClusterConfigCM_NewFormat(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "topology", Namespace: "gpu-operator"},
		Data: map[string]string{
			CmTopologyKey: `
nodePoolLabelKey: run.ai/simulated-gpu-node-pool
migStrategy: mixed
nodePools:
  pool-a:
    gpu:
      backend: fake
      profile: h100
`,
		},
	}

	config, err := FromClusterConfigCM(cm)
	require.NoError(t, err)
	assert.Equal(t, "h100", config.NodePools["pool-a"].Gpu.Profile)
}

func TestFromClusterConfigCM_MissingKey(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "topology", Namespace: "gpu-operator"},
		Data:       map[string]string{},
	}

	_, err := FromClusterConfigCM(cm)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing key")
}
