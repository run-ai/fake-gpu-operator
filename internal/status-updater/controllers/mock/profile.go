package mock

import (
	"fmt"

	commonprofile "github.com/run-ai/fake-gpu-operator/internal/common/profile"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

// RenderConfig produces the serialized YAML body for a per-pool nvml-mock
// ConfigMap's `config.yaml` key, plus the GpuSpec extracted from the merged
// profile (used by the DaemonSet builder for literal env vars). It loads the
// profile ConfigMap referenced by gpu.Profile, deep-merges gpu.Overrides on
// top using the existing common/profile helpers, then serializes and
// extracts in one pass.
//
// Returns an error if the profile name is empty, the profile ConfigMap is
// missing, or the profile YAML is malformed.
func RenderConfig(kube kubernetes.Interface, namespace string, gpu topology.GpuConfig) ([]byte, commonprofile.GpuSpec, error) {
	if gpu.Profile == "" {
		return nil, commonprofile.GpuSpec{}, fmt.Errorf("mock pool requires a non-empty profile")
	}

	base, err := commonprofile.Load(kube, namespace, gpu.Profile)
	if err != nil {
		return nil, commonprofile.GpuSpec{}, fmt.Errorf("loading profile %q: %w", gpu.Profile, err)
	}

	merged := commonprofile.Merge(base, gpu.Overrides)

	out, err := yaml.Marshal(merged)
	if err != nil {
		return nil, commonprofile.GpuSpec{}, fmt.Errorf("serializing merged profile: %w", err)
	}
	return out, commonprofile.Extract(merged), nil
}
