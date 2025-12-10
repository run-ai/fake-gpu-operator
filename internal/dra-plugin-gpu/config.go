package dra_plugin_gpu

import (
	"path/filepath"

	coreclientset "k8s.io/client-go/kubernetes"
)

const (
	DriverName = "gpu.nvidia.com" // Override driver name for deviceclass compatibility
)

// Flags contains configuration flags for the DRA plugin
type Flags struct {
	NodeName                      string `mapstructure:"NODE_NAME" validator:"required"`
	CDIRoot                       string `mapstructure:"CDI_ROOT"`
	KubeletRegistrarDirectoryPath string `mapstructure:"KUBELET_REGISTRAR_DIRECTORY_PATH"`
	KubeletPluginsDirectoryPath   string `mapstructure:"KUBELET_PLUGINS_DIRECTORY_PATH"`
	HealthcheckPort               int    `mapstructure:"HEALTHCHECK_PORT"`
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
