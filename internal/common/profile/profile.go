package profile

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GpuSpec holds the extracted GPU specification fields that FGO components need.
type GpuSpec struct {
	GpuProduct    string
	GpuMemory     int // MiB
	GpuCount      int
	Architecture  string
	DriverVersion string
	CudaVersion   string
}

// Load reads a GPU profile ConfigMap by name and returns the parsed profile data.
// The ConfigMap is expected to be named "gpu-profile-{name}" in the given namespace.
func Load(kubeClient kubernetes.Interface, namespace, profileName string) (map[string]interface{}, error) {
	cmName := CmNamePrefix + profileName
	cm, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(
		context.TODO(), cmName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to load GPU profile %q: %w", profileName, err)
	}

	data, ok := cm.Data[CmProfileKey]
	if !ok {
		return nil, fmt.Errorf("GPU profile ConfigMap %q missing key %q", cmName, CmProfileKey)
	}

	var profile map[string]interface{}
	if err := yaml.Unmarshal([]byte(data), &profile); err != nil {
		return nil, fmt.Errorf("failed to parse GPU profile %q: %w", profileName, err)
	}

	return profile, nil
}

// Merge deep-merges overrides onto a base profile. Scalars and lists are replaced;
// nested maps are recursively merged. Returns a new map (base is not modified).
func Merge(base, overrides map[string]interface{}) map[string]interface{} {
	if len(overrides) == 0 {
		return copyMap(base)
	}

	result := copyMap(base)
	for key, overrideVal := range overrides {
		baseVal, exists := result[key]
		if !exists {
			result[key] = overrideVal
			continue
		}

		baseMap, baseIsMap := baseVal.(map[string]interface{})
		overrideMap, overrideIsMap := overrideVal.(map[string]interface{})
		if baseIsMap && overrideIsMap {
			result[key] = Merge(baseMap, overrideMap)
		} else {
			result[key] = overrideVal
		}
	}

	return result
}

// Extract pulls FGO-relevant fields from a resolved profile into a GpuSpec.
// Missing fields are returned as zero values.
func Extract(profile map[string]interface{}) GpuSpec {
	var spec GpuSpec
	if profile == nil {
		return spec
	}

	if dd, ok := getMap(profile, "device_defaults"); ok {
		spec.GpuProduct, _ = dd["name"].(string)
		spec.Architecture, _ = dd["architecture"].(string)

		if mem, ok := getMap(dd, "memory"); ok {
			spec.GpuMemory = toMiB(mem["total_bytes"])
		}
	}

	if sys, ok := getMap(profile, "system"); ok {
		spec.DriverVersion, _ = sys["driver_version"].(string)
		spec.CudaVersion, _ = sys["cuda_version"].(string)
	}

	spec.GpuCount = DeviceCount(profile)

	return spec
}

func copyMap(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func getMap(m map[string]interface{}, key string) (map[string]interface{}, bool) {
	val, ok := m[key]
	if !ok {
		return nil, false
	}
	asMap, ok := val.(map[string]interface{})
	return asMap, ok
}

// DeviceCount returns the number of GPU devices in a profile.
// It checks for an explicit "device_count" field first, then falls back to
// counting the "devices" list.
func DeviceCount(profile map[string]interface{}) int {
	var count int
	switch n := profile["device_count"].(type) {
	case int:
		count = n
	case int64:
		count = int(n)
	case float64:
		count = int(n)
	}
	if count > 0 {
		return count
	}
	if devices, ok := profile["devices"].([]interface{}); ok {
		return len(devices)
	}
	return 0
}

// toMiB converts a bytes value (which may be int, int64, or float64 from YAML
// unmarshaling) to MiB.
func toMiB(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n / (1024 * 1024)
	case int64:
		return int(n / (1024 * 1024))
	case float64:
		return int(n / (1024 * 1024))
	default:
		return 0
	}
}
