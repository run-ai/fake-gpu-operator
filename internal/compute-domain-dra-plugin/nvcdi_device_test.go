package computedomaindraplugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeDomainNvcdiDevice(t *testing.T) {
	tmpDir := t.TempDir()
	deviceRoot := filepath.Join(tmpDir, "devices")

	device, err := newComputeDomainNvcdiDevice(deviceRoot)
	require.NoError(t, err)

	info := &DomainInfo{
		DomainID: "domain-123",
		Claims:   []string{},
	}

	edits, err := device.ContainerEdits(info)
	require.NoError(t, err)
	require.NotNil(t, edits)
	require.NotNil(t, edits.ContainerEdits)
	require.Len(t, edits.DeviceNodes, 1)

	node := edits.DeviceNodes[0]
	assert.Contains(t, node.Path, "channel-0")
	assert.Equal(t, "c", node.Type)

	// Ensure the symlink exists and points to /dev/null
	target, err := os.Readlink(node.Path)
	require.NoError(t, err)
	assert.Equal(t, defaultHostDeviceTarget, target)
}
