package topology

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
