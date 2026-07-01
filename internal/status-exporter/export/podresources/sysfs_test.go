package podresources

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/numazones"
)

func TestRenderCpulist(t *testing.T) {
	root := t.TempDir()
	cpu := resource.MustParse("4")
	layout := &numazones.ZoneLayout{
		Zones: 2, GpusPerZone: []int{2, 2}, CPUPerZone: &cpu,
		CPUIDsPerZone: [][]int64{{0, 1, 2, 3}, {4, 5, 6, 7}},
	}
	require.NoError(t, RenderCpulist(root, layout))

	b0, err := os.ReadFile(filepath.Join(root, "devices/system/node/node0/cpulist"))
	require.NoError(t, err)
	assert.Equal(t, "0-3\n", string(b0))
	b1, err := os.ReadFile(filepath.Join(root, "devices/system/node/node1/cpulist"))
	require.NoError(t, err)
	assert.Equal(t, "4-7\n", string(b1))
}

func TestRenderCpulist_PurgesStale(t *testing.T) {
	root := t.TempDir()
	stale := filepath.Join(root, "devices/system/node/node9")
	require.NoError(t, os.MkdirAll(stale, 0755))
	cpu := resource.MustParse("2")
	layout := &numazones.ZoneLayout{Zones: 1, GpusPerZone: []int{1}, CPUPerZone: &cpu, CPUIDsPerZone: [][]int64{{0, 1}}}
	require.NoError(t, RenderCpulist(root, layout))
	_, err := os.Stat(stale)
	assert.True(t, os.IsNotExist(err), "stale node9 dir should be purged")
}
