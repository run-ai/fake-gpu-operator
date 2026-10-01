package topology

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNodeTopologyServerURL(t *testing.T) {
	assert.Equal(t, "http://topology-server.fake-gpu-operator/topology/nodes/worker-1",
		NodeTopologyServerURL("fake-gpu-operator", "worker-1"))
}

func TestNodeTopologyServerURL_DefaultsNamespace(t *testing.T) {
	assert.Equal(t, "http://topology-server.gpu-operator/topology/nodes/worker-1",
		NodeTopologyServerURL("", "worker-1"))
}
