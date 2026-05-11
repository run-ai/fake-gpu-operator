package mock

import (
	"testing"

	commonprofile "github.com/run-ai/fake-gpu-operator/internal/common/profile"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func mkProfileCM(name, ns string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    map[string]string{commonprofile.LabelGpuProfile: "true"},
		},
		Data: map[string]string{
			commonprofile.CmProfileKey: `system:
  driver_version: "550"`,
		},
	}
}

func TestComputeMockPoolResources_OnlyMockPools(t *testing.T) {
	kube := fake.NewSimpleClientset(mkProfileCM("gpu-profile-a100", "ns"))
	cfg := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools: map[string]topology.NodePoolConfig{
			"fake-pool":      {Gpu: topology.GpuConfig{Backend: "fake", Profile: "a100"}},
			"mock-train":     {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
			"mock-inference": {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
		},
	}
	params := ReconcileParams{
		Namespace:       "ns",
		Image:           "ghcr.io/nvidia/nvml-mock:v0.1.0",
		ImagePullPolicy: corev1.PullIfNotPresent,
	}

	configMaps, daemonSets, err := ComputeMockPoolResources(kube, cfg, params)
	require.NoError(t, err)

	// Two mock pools × (1 CM + 1 DS); fake pool produces neither.
	assert.Len(t, configMaps, 2, "one CM per mock pool")
	assert.Len(t, daemonSets, 2, "one DS per mock pool")

	cmNames := map[string]bool{}
	for _, cm := range configMaps {
		cmNames[cm.Name] = true
	}
	dsNames := map[string]bool{}
	for _, ds := range daemonSets {
		dsNames[ds.Name] = true
	}
	assert.True(t, cmNames["nvml-mock-mock-train"])
	assert.True(t, cmNames["nvml-mock-mock-inference"])
	assert.True(t, dsNames["nvml-mock-mock-train"])
	assert.True(t, dsNames["nvml-mock-mock-inference"])
	assert.False(t, dsNames["nvml-mock-fake-pool"])
}

func TestComputeMockPoolResources_DeterministicOrder(t *testing.T) {
	kube := fake.NewSimpleClientset(mkProfileCM("gpu-profile-a100", "ns"))
	cfg := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools: map[string]topology.NodePoolConfig{
			"zzz": {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
			"aaa": {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
			"mmm": {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
		},
	}
	params := ReconcileParams{Namespace: "ns", Image: "img:t"}

	configMaps, daemonSets, err := ComputeMockPoolResources(kube, cfg, params)
	require.NoError(t, err)

	require.Len(t, configMaps, 3)
	require.Len(t, daemonSets, 3)
	assert.Equal(t, "nvml-mock-aaa", configMaps[0].Name)
	assert.Equal(t, "nvml-mock-mmm", configMaps[1].Name)
	assert.Equal(t, "nvml-mock-zzz", configMaps[2].Name)
	assert.Equal(t, "nvml-mock-aaa", daemonSets[0].Name)
	assert.Equal(t, "nvml-mock-mmm", daemonSets[1].Name)
	assert.Equal(t, "nvml-mock-zzz", daemonSets[2].Name)
}

func TestComputeMockPoolResources_PropagatesProfileError(t *testing.T) {
	// No profile CMs in cluster → load fails for the mock pool.
	kube := fake.NewSimpleClientset()
	cfg := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools: map[string]topology.NodePoolConfig{
			"mock-pool": {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
		},
	}
	_, _, err := ComputeMockPoolResources(kube, cfg, ReconcileParams{Namespace: "ns"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock-pool")
}
