package mock

import (
	"testing"

	commonprofile "github.com/run-ai/fake-gpu-operator/internal/common/profile"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
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

func TestComputeDesiredState_OnlyMockPools(t *testing.T) {
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

	objs, err := ComputeDesiredState(kube, cfg, params)
	require.NoError(t, err)

	// 2 mock pools × (1 DS + 1 CM) = 4 resources; fake pool produces 0.
	assert.Len(t, objs, 4, "exactly two DS+CM pairs for the two mock pools")

	dsNames, cmNames := map[string]bool{}, map[string]bool{}
	for _, o := range objs {
		switch r := o.(type) {
		case *appsv1.DaemonSet:
			dsNames[r.Name] = true
		case *corev1.ConfigMap:
			cmNames[r.Name] = true
		}
	}
	assert.True(t, dsNames["nvml-mock-mock-train"])
	assert.True(t, dsNames["nvml-mock-mock-inference"])
	assert.True(t, cmNames["nvml-mock-mock-train"])
	assert.True(t, cmNames["nvml-mock-mock-inference"])
	assert.False(t, dsNames["nvml-mock-fake-pool"])
}

func TestComputeDesiredState_DeterministicOrder(t *testing.T) {
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

	objs, err := ComputeDesiredState(kube, cfg, params)
	require.NoError(t, err)

	// Pool order should be sorted: aaa, mmm, zzz. Each pool emits CM then DS.
	require.GreaterOrEqual(t, len(objs), 6)
	cm0 := objs[0].(*corev1.ConfigMap)
	cm2 := objs[2].(*corev1.ConfigMap)
	cm4 := objs[4].(*corev1.ConfigMap)
	assert.Equal(t, "nvml-mock-aaa", cm0.Name)
	assert.Equal(t, "nvml-mock-mmm", cm2.Name)
	assert.Equal(t, "nvml-mock-zzz", cm4.Name)
}

func TestComputeDesiredState_PropagatesProfileError(t *testing.T) {
	// No profile CMs in cluster → load fails for the mock pool.
	kube := fake.NewSimpleClientset()
	cfg := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools: map[string]topology.NodePoolConfig{
			"mock-pool": {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
		},
	}
	_, err := ComputeDesiredState(kube, cfg, ReconcileParams{Namespace: "ns"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock-pool")
}
