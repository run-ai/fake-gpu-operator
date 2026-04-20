package topology

import (
	"fmt"

	"github.com/run-ai/fake-gpu-operator/internal/common/profile"
	"k8s.io/client-go/kubernetes"
)

// ResolvedPool holds the GPU spec fields extracted from a NodePoolConfig after
// profile resolution (Load → Merge → Extract). Downstream code uses these
// fields to build NodeTopology ConfigMaps.
type ResolvedPool struct {
	GpuProduct   string
	GpuMemory    int // MiB
	GpuCount     int
	OtherDevices []GenericDevice
}

// ResolveNodePool resolves a NodePoolConfig into concrete GPU spec fields.
// If the pool references a profile, it is loaded from the cluster, merged with
// overrides, and extracted. If no profile is set, the overrides are used directly
// (backwards-compatible path from normalization).
func ResolveNodePool(kubeClient kubernetes.Interface, namespace string, pool NodePoolConfig) (*ResolvedPool, error) {
	resolved := &ResolvedPool{}

	// Map resources to OtherDevices (everything except nvidia.com/gpu)
	for _, res := range pool.Resources {
		for name, count := range res {
			if name == "nvidia.com/gpu" {
				continue
			}
			resolved.OtherDevices = append(resolved.OtherDevices, GenericDevice{
				Name:  name,
				Count: count,
			})
		}
	}

	if pool.Gpu.Profile != "" {
		return resolveWithProfile(kubeClient, namespace, pool, resolved)
	}

	return resolveFromOverrides(pool, resolved)
}

// resolveWithProfile loads a profile ConfigMap, merges overrides, and extracts GPU spec.
func resolveWithProfile(kubeClient kubernetes.Interface, namespace string, pool NodePoolConfig, resolved *ResolvedPool) (*ResolvedPool, error) {
	base, err := profile.Load(kubeClient, namespace, pool.Gpu.Profile)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve profile %q: %w", pool.Gpu.Profile, err)
	}

	merged := profile.Merge(base, pool.Gpu.Overrides)
	spec := profile.Extract(merged)

	resolved.GpuProduct = spec.GpuProduct
	resolved.GpuMemory = spec.GpuMemory
	resolved.GpuCount = spec.GpuCount

	return resolved, nil
}

// resolveFromOverrides extracts GPU spec directly from overrides.
// This is the backwards-compatible path used when normalization converts
// old-format fields (gpuProduct, gpuMemory, gpuCount) into overrides.
func resolveFromOverrides(pool NodePoolConfig, resolved *ResolvedPool) (*ResolvedPool, error) {
	overrides := pool.Gpu.Overrides
	if overrides == nil {
		return resolved, nil
	}

	if dd, ok := overrides["device_defaults"].(map[string]interface{}); ok {
		resolved.GpuProduct, _ = dd["name"].(string)
		if mem, ok := dd["memory"].(map[string]interface{}); ok {
			// Normalization stores total_bytes (int64); convert to MiB
			if totalBytes, ok := mem["total_bytes"]; ok {
				switch v := totalBytes.(type) {
				case int:
					resolved.GpuMemory = v / (1024 * 1024)
				case int64:
					resolved.GpuMemory = int(v / (1024 * 1024))
				case float64:
					resolved.GpuMemory = int(v / (1024 * 1024))
				}
			}
		}
	}

	resolved.GpuCount = profile.DeviceCount(overrides)

	return resolved, nil
}
