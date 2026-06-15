package nrt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

func allocatable(cpu, mem string) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(cpu),
		corev1.ResourceMemory: resource.MustParse(mem),
	}
}

func TestBuildNRT_TwoZonesEvenSplit(t *testing.T) {
	numa := topology.NumaConfig{Zones: 2}
	got, err := BuildNRT("node-a", numa, 8, allocatable("8", "128Gi"))
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "node-a", got.Name)
	assert.Equal(t, "topology.node.k8s.io/v1alpha2", got.APIVersion)
	assert.Equal(t, "NodeResourceTopology", got.Kind)
	require.Len(t, got.Zones, 2)

	attrs := map[string]string{}
	for _, a := range got.Attributes {
		attrs[a.Name] = a.Value
	}
	assert.Equal(t, "single-numa-node", attrs["topologyManagerPolicy"])
	assert.Equal(t, "container", attrs["topologyManagerScope"])
	assert.Equal(t, []string{"SingleNUMANodeContainerLevel"}, got.TopologyPolicies)

	z0 := got.Zones[0]
	assert.Equal(t, "node-0", z0.Name)
	assert.Equal(t, "Node", z0.Type)

	costs := map[string]int64{}
	for _, c := range z0.Costs {
		costs[c.Name] = c.Value
	}
	assert.Equal(t, int64(10), costs["node-0"])
	assert.Equal(t, int64(21), costs["node-1"])

	res := map[string]resource.Quantity{}
	for _, r := range z0.Resources {
		res[r.Name] = r.Available
		assert.Equal(t, 0, r.Capacity.Cmp(r.Available), "capacity==available for %s", r.Name)
		assert.Equal(t, 0, r.Allocatable.Cmp(r.Available), "allocatable==available for %s", r.Name)
	}
	gpu := res["nvidia.com/gpu"]
	assert.Equal(t, int64(4), (&gpu).Value())
	cpu := res["cpu"]
	assert.Equal(t, int64(4), (&cpu).Value())
	mem := res["memory"]
	expected64Gi := resource.MustParse("64Gi")
	assert.Equal(t, (&expected64Gi).Value(), (&mem).Value())
}

func TestBuildNRT_ExplicitUnevenAndCustomDistances(t *testing.T) {
	numa := topology.NumaConfig{
		Zones:       2,
		GpusPerZone: []int{6, 2},
		Distances:   &topology.NumaDistances{Self: 10, Remote: 30},
	}
	got, err := BuildNRT("node-b", numa, 8, allocatable("8", "128Gi"))
	require.NoError(t, err)
	require.Len(t, got.Zones, 2)

	gpu := func(z int) int64 {
		for _, r := range got.Zones[z].Resources {
			if r.Name == "nvidia.com/gpu" {
				return r.Available.Value()
			}
		}
		return -1
	}
	assert.Equal(t, int64(6), gpu(0))
	assert.Equal(t, int64(2), gpu(1))

	var remote int64
	for _, c := range got.Zones[0].Costs {
		if c.Name == "node-1" {
			remote = c.Value
		}
	}
	assert.Equal(t, int64(30), remote)
}

func TestBuildNRT_GPUOnlyWhenNoAllocatableNoOverride(t *testing.T) {
	got, err := BuildNRT("node-c", topology.NumaConfig{Zones: 2}, 8, corev1.ResourceList{})
	require.NoError(t, err)
	for _, z := range got.Zones {
		require.Len(t, z.Resources, 1)
		assert.Equal(t, "nvidia.com/gpu", z.Resources[0].Name)
	}
}

func TestBuildNRT_PodScopeLegacyPolicy(t *testing.T) {
	numa := topology.NumaConfig{Zones: 1, TopologyManagerScope: "pod"}
	got, err := BuildNRT("node-d", numa, 4, corev1.ResourceList{})
	require.NoError(t, err)
	assert.Equal(t, []string{"SingleNUMANodePodLevel"}, got.TopologyPolicies)
}

func TestBuildNRT_ZonesBelowOneIsNil(t *testing.T) {
	got, err := BuildNRT("node-e", topology.NumaConfig{Zones: 0}, 8, corev1.ResourceList{})
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestBuildNRT_MismatchedGpusPerZoneErrors(t *testing.T) {
	_, err := BuildNRT("node-f", topology.NumaConfig{Zones: 2, GpusPerZone: []int{5, 2}}, 8, corev1.ResourceList{})
	require.Error(t, err)
}

func TestBuildNRT_InvalidCPUPerZoneErrors(t *testing.T) {
	_, err := BuildNRT("node-g", topology.NumaConfig{Zones: 2, CPUPerZone: "notaquantity"}, 8, allocatable("8", "128Gi"))
	require.Error(t, err)
}
