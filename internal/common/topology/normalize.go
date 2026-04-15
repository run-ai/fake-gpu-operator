package topology

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// normalizeOldToClusterConfig converts the old ClusterTopology format into the
// new ClusterConfig format. This enables backwards compatibility: users can keep
// using the old topology: values.yaml format and it gets normalized at read time.
func normalizeOldToClusterConfig(old *ClusterTopology) *ClusterConfig {
	config := &ClusterConfig{
		NodePoolLabelKey: old.NodePoolLabelKey,
		MigStrategy:      old.MigStrategy,
		NodePools:        make(map[string]NodePoolConfig, len(old.NodePools)),
	}

	for name, pool := range old.NodePools {
		config.NodePools[name] = normalizeNodePool(pool)
	}

	return config
}

func normalizeNodePool(old NodePoolTopology) NodePoolConfig {
	deviceDefaults := map[string]interface{}{
		"name": old.GpuProduct,
		"memory": map[string]interface{}{
			"total_bytes": int64(old.GpuMemory) * 1024 * 1024,
		},
	}

	devices := make([]interface{}, old.GpuCount)
	for i := range devices {
		devices[i] = map[string]interface{}{}
	}

	overrides := map[string]interface{}{
		"device_defaults": deviceDefaults,
		"devices":         devices,
	}

	poolConfig := NodePoolConfig{
		Gpu: GpuConfig{
			Backend:   "fake",
			Overrides: overrides,
		},
	}

	if len(old.OtherDevices) > 0 {
		resources := make([]map[string]int, len(old.OtherDevices))
		for i, dev := range old.OtherDevices {
			resources[i] = map[string]int{dev.Name: dev.Count}
		}
		poolConfig.Resources = resources
	}

	return poolConfig
}

// ParseAndNormalizeTopology takes raw YAML bytes from the topology ConfigMap and
// returns a ClusterConfig. It auto-detects whether the data is in old (ClusterTopology)
// or new (ClusterConfig) format.
func ParseAndNormalizeTopology(data []byte) (*ClusterConfig, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("topology data is empty")
	}

	if isNewFormat(data) {
		var config ClusterConfig
		if err := yaml.Unmarshal(data, &config); err != nil {
			return nil, fmt.Errorf("failed to parse new format topology: %w", err)
		}
		return &config, nil
	}

	var old ClusterTopology
	if err := yaml.Unmarshal(data, &old); err != nil {
		return nil, fmt.Errorf("failed to parse old format topology: %w", err)
	}
	return normalizeOldToClusterConfig(&old), nil
}

// isNewFormat checks if the YAML contains new-format markers.
// New format has nodePools with nested gpu.backend fields.
// Old format has nodePools with flat gpuProduct/gpuCount/gpuMemory fields.
func isNewFormat(data []byte) bool {
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false
	}

	nodePools, ok := raw["nodePools"]
	if !ok {
		return false
	}

	poolsMap, ok := nodePools.(map[string]interface{})
	if !ok {
		return false
	}

	for _, pool := range poolsMap {
		poolMap, ok := pool.(map[string]interface{})
		if !ok {
			continue
		}
		// If any pool has a "gpu" key, it's new format
		if _, hasGpu := poolMap["gpu"]; hasGpu {
			return true
		}
		// If any pool has "gpuProduct" or "gpuCount", it's old format
		if _, hasProduct := poolMap["gpuProduct"]; hasProduct {
			return false
		}
		if _, hasCount := poolMap["gpuCount"]; hasCount {
			return false
		}
	}

	// No discriminating keys found — check for gpuOperator key (new format only)
	if _, hasOperator := raw["gpuOperator"]; hasOperator {
		return true
	}

	// Default to old format for backwards compat
	return false
}
