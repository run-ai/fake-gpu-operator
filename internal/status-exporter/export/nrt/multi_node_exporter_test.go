package nrt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMultiNodeExporter_SetNRTForNode(t *testing.T) {
	client := seedCluster(t, numaClusterYAML, gpuNode("kwok-0", "default"))
	pub := &recordingPublisher{}
	e := NewMultiNodeExporter(client, pub)

	require.NoError(t, e.SetNRTForNode("kwok-0", topo(8)))

	require.Len(t, pub.applied, 1)
	assert.Equal(t, "kwok-0", pub.applied[0].Name)
	require.Len(t, pub.applied[0].OwnerReferences, 1)
}
