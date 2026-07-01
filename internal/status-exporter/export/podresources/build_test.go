package podresources

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/numazones"
)

func gpu(id, ns, pod string) topology.GpuDetails {
	return topology.GpuDetails{ID: id, Status: topology.GpuStatus{
		AllocatedBy: topology.ContainerDetails{Namespace: ns, Pod: pod},
	}}
}

// layout: 2 zones, 4 gpus/zone, 8 cores/zone (ids 0-7 / 8-15).
func layout2z() *numazones.ZoneLayout {
	cpu := resource.MustParse("8")
	mem := resource.MustParse("16Gi")
	return &numazones.ZoneLayout{
		Zones: 2, GpusPerZone: []int{4, 4},
		CPUPerZone: &cpu, MemPerZone: &mem,
		CPUIDsPerZone: [][]int64{{0, 1, 2, 3, 4, 5, 6, 7}, {8, 9, 10, 11, 12, 13, 14, 15}},
	}
}

func TestBuildPodResources_SingleZonePod(t *testing.T) {
	nt := &topology.NodeTopology{Gpus: []topology.GpuDetails{
		gpu("g0", "ns", "p1"), gpu("g1", "ns", "p1"), // both in zone 0 (indexes 0,1)
		gpu("g2", "", ""), gpu("g3", "", ""),
		gpu("g4", "", ""), gpu("g5", "", ""), gpu("g6", "", ""), gpu("g7", "", ""),
	}}
	reqs := map[string]PodRequest{"ns/p1": {CPU: resource.MustParse("4"), Memory: resource.MustParse("8Gi")}}

	got := BuildPodResources(nt, layout2z(), reqs)
	require.Len(t, got, 1)
	assert.Equal(t, "p1", got[0].Name)
	assert.Equal(t, "ns", got[0].Namespace)
	require.Len(t, got[0].Containers, 1)
	c := got[0].Containers[0]

	// One device entry, zone 0, 2 GPUs.
	require.Len(t, c.Devices, 1)
	assert.Equal(t, "nvidia.com/gpu", c.Devices[0].ResourceName)
	assert.Len(t, c.Devices[0].DeviceIds, 2)
	require.Len(t, c.Devices[0].Topology.Nodes, 1)
	assert.Equal(t, int64(0), c.Devices[0].Topology.Nodes[0].ID)

	// 4 cpu cores, all in zone 0 (ids 0..3).
	assert.Equal(t, []int64{0, 1, 2, 3}, c.CpuIds)

	// 8Gi memory, zone 0, size > 0.
	require.Len(t, c.Memory, 1)
	assert.Equal(t, "memory", c.Memory[0].MemoryType)
	mem := resource.MustParse("8Gi")
	assert.Equal(t, uint64(mem.Value()), c.Memory[0].Size)
	assert.Equal(t, int64(0), c.Memory[0].Topology.Nodes[0].ID)
}

func TestBuildPodResources_SpansTwoZones(t *testing.T) {
	// p1 holds gpu index 3 (zone0) and index 4 (zone1) -> 1 gpu per zone.
	gpus := make([]topology.GpuDetails, 8)
	for i := range gpus {
		gpus[i] = gpu("g", "", "")
	}
	gpus[3] = gpu("g3", "ns", "p1")
	gpus[4] = gpu("g4", "ns", "p1")
	nt := &topology.NodeTopology{Gpus: gpus}
	reqs := map[string]PodRequest{"ns/p1": {CPU: resource.MustParse("4"), Memory: resource.MustParse("8Gi")}}

	got := BuildPodResources(nt, layout2z(), reqs)
	require.Len(t, got, 1)
	c := got[0].Containers[0]

	// Two device entries, one per zone, sorted by zone.
	require.Len(t, c.Devices, 2)
	zones := []int64{c.Devices[0].Topology.Nodes[0].ID, c.Devices[1].Topology.Nodes[0].ID}
	assert.Equal(t, []int64{0, 1}, zones)

	// 4 cores split 2/2 across the two zones: ids 0,1 (zone0) and 8,9 (zone1).
	sort.Slice(c.CpuIds, func(i, j int) bool { return c.CpuIds[i] < c.CpuIds[j] })
	assert.Equal(t, []int64{0, 1, 8, 9}, c.CpuIds)

	// Two memory entries, 4Gi each.
	require.Len(t, c.Memory, 2)
}

func TestBuildPodResources_NoGPUsNoEntries(t *testing.T) {
	nt := &topology.NodeTopology{Gpus: []topology.GpuDetails{gpu("g0", "", ""), gpu("g1", "", "")}}
	assert.Empty(t, BuildPodResources(nt, layout2z(), nil))
}
