package topology

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestGpuDetails_NUMANodeYAML(t *testing.T) {
	// nil NUMANode is omitted from YAML
	out, err := yaml.Marshal(GpuDetails{ID: "g0"})
	require.NoError(t, err)
	assert.NotContains(t, string(out), "numaNode")

	// set NUMANode is marshaled and round-trips
	n := 1
	out, err = yaml.Marshal(GpuDetails{ID: "g0", NUMANode: &n})
	require.NoError(t, err)
	assert.Contains(t, string(out), "numaNode: 1")

	var back GpuDetails
	require.NoError(t, yaml.Unmarshal(out, &back))
	require.NotNil(t, back.NUMANode)
	assert.Equal(t, 1, *back.NUMANode)

	// NUMANode 0 must NOT be omitted (zero is a valid NUMA node; omitempty keys off nil, not value)
	n0 := 0
	out0, err := yaml.Marshal(GpuDetails{ID: "g0", NUMANode: &n0})
	require.NoError(t, err)
	assert.Contains(t, string(out0), "numaNode: 0")

	var back0 GpuDetails
	require.NoError(t, yaml.Unmarshal(out0, &back0))
	require.NotNil(t, back0.NUMANode)
	assert.Equal(t, 0, *back0.NUMANode)
}
