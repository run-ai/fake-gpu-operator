package numazones

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

func alloc(cpu, mem string) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(cpu),
		corev1.ResourceMemory: resource.MustParse(mem),
	}
}

func TestResolveZoneLayout_EvenSplit(t *testing.T) {
	l, err := ResolveZoneLayout(topology.NumaConfig{Zones: 2}, 8, alloc("32", "128Gi"))
	require.NoError(t, err)
	require.NotNil(t, l)
	assert.Equal(t, 2, l.Zones)
	assert.Equal(t, []int{4, 4}, l.GpusPerZone)
	require.NotNil(t, l.CPUPerZone)
	assert.Equal(t, int64(16), l.CPUPerZone.Value()) // 32 / 2 zones
	// 16 cores/zone: zone0 ids 0..15, zone1 ids 16..31
	assert.Equal(t, int64(0), l.CPUIDsPerZone[0][0])
	assert.Len(t, l.CPUIDsPerZone[0], 16)
	assert.Equal(t, int64(16), l.CPUIDsPerZone[1][0])
	assert.Len(t, l.CPUIDsPerZone[1], 16)
}

func TestResolveZoneLayout_Disabled(t *testing.T) {
	l, err := ResolveZoneLayout(topology.NumaConfig{Zones: 0}, 8, alloc("32", "128Gi"))
	require.NoError(t, err)
	assert.Nil(t, l)
}

func TestGPUIndexToZone(t *testing.T) {
	assert.Equal(t, []int{0, 0, 0, 0, 1, 1, 1, 1}, GPUIndexToZone([]int{4, 4}))
	assert.Equal(t, []int{0, 0, 0, 1}, GPUIndexToZone([]int{3, 1}))
}

func TestCpulist(t *testing.T) {
	assert.Equal(t, "0-3", Cpulist([]int64{0, 1, 2, 3}))
	assert.Equal(t, "0-1,4", Cpulist([]int64{0, 1, 4}))
	assert.Equal(t, "", Cpulist(nil))
}
