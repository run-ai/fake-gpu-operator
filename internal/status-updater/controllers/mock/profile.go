package mock

import (
	"fmt"

	commonprofile "github.com/run-ai/fake-gpu-operator/internal/common/profile"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

// RenderConfig produces the serialized YAML body for a per-pool nvml-mock
// ConfigMap's `config.yaml` key. It loads the profile ConfigMap referenced
// by gpu.Profile, deep-merges gpu.Overrides on top using the existing
// common/profile helpers, and serializes the result.
//
// Returns an error if the profile name is empty, the profile ConfigMap is
// missing, or the profile YAML is malformed.
func RenderConfig(kube kubernetes.Interface, namespace string, gpu topology.GpuConfig) ([]byte, error) {
	if gpu.Profile == "" {
		return nil, fmt.Errorf("mock pool requires a non-empty profile")
	}

	base, err := commonprofile.Load(kube, namespace, gpu.Profile)
	if err != nil {
		return nil, fmt.Errorf("loading profile %q: %w", gpu.Profile, err)
	}

	merged := commonprofile.Merge(base, gpu.Overrides)

	out, err := yaml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("serializing merged profile: %w", err)
	}
	return out, nil
}
