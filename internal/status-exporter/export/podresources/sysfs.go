package podresources

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/numazones"
)

// RenderCpulist writes <root>/devices/system/node/node<z>/cpulist for every zone, in the
// Linux cpulist format KAI's cputopology parser reads. The node subtree is purged first so
// a shrunk zone count leaves no stale directories.
func RenderCpulist(root string, layout *numazones.ZoneLayout) error {
	if layout == nil {
		return nil
	}
	base := filepath.Join(root, "devices", "system", "node")
	if err := os.RemoveAll(base); err != nil {
		return fmt.Errorf("purge sysfs tree: %w", err)
	}
	for z := 0; z < layout.Zones; z++ {
		dir := filepath.Join(base, fmt.Sprintf("node%d", z))
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
		content := numazones.Cpulist(layout.CPUIDsPerZone[z])
		if err := os.WriteFile(filepath.Join(dir, "cpulist"), []byte(content+"\n"), 0644); err != nil {
			return fmt.Errorf("write cpulist for zone %d: %w", z, err)
		}
	}
	return nil
}
