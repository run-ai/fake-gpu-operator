package dra_plugin_gpu

import (
	"path/filepath"

	coreclientset "k8s.io/client-go/kubernetes"
)

const (
	DriverPluginCheckpointFile = "checkpoint.json"
	DriverName                 = "gpu.nvidia.com" // Override driver name for deviceclass compatibility
)

// Flags contains configuration flags for the DRA plugin
type Flags struct {
	NodeName                      string
	CDIRoot                       string
	KubeletRegistrarDirectoryPath string
	KubeletPluginsDirectoryPath   string
	HealthcheckPort               int
}

// Config contains the configuration for the DRA plugin
type Config struct {
	Flags         *Flags
	CoreClient    coreclientset.Interface
	CancelMainCtx func(error)
}

// DriverPluginPath returns the path to the driver plugin directory
func (c *Config) DriverPluginPath() string {
	return filepath.Join(c.Flags.KubeletPluginsDirectoryPath, DriverName)
}
