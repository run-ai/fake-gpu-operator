package topology

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestNodePoolConfigUnmarshalsNuma(t *testing.T) {
	const data = `
gpu:
  backend: fake
  profile: a100
numa:
  zones: 2
  gpusPerZone: [4, 4]
  topologyManagerPolicy: single-numa-node
  topologyManagerScope: container
  cpuPerZone: "4"
  memPerZone: 64Gi
  distances:
    self: 10
    remote: 21
`
	var pool NodePoolConfig
	require.NoError(t, yaml.Unmarshal([]byte(data), &pool))

	require.NotNil(t, pool.Numa)
	assert.Equal(t, 2, pool.Numa.Zones)
	assert.Equal(t, []int{4, 4}, pool.Numa.GpusPerZone)
	assert.Equal(t, "single-numa-node", pool.Numa.TopologyManagerPolicy)
	assert.Equal(t, "container", pool.Numa.TopologyManagerScope)
	assert.Equal(t, "4", pool.Numa.CPUPerZone)
	assert.Equal(t, "64Gi", pool.Numa.MemPerZone)
	require.NotNil(t, pool.Numa.Distances)
	assert.Equal(t, 10, pool.Numa.Distances.Self)
	assert.Equal(t, 21, pool.Numa.Distances.Remote)
}

func TestNodePoolConfigNumaOmittedIsNil(t *testing.T) {
	var pool NodePoolConfig
	require.NoError(t, yaml.Unmarshal([]byte("gpu:\n  backend: fake\n  profile: a100\n"), &pool))
	assert.Nil(t, pool.Numa)
}
