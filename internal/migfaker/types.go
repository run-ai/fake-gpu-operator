package migfaker

type AnnotationMigConfig struct {
	Version    string     `yaml:"version"`
	MigConfigs MigConfigs `yaml:"mig-configs"`
}

type MigConfigs struct {
	SelectedDevices []SelectedDevices `yaml:"selected"`
}

type SelectedDevices struct {
	Devices    []string    `yaml:"devices"`
	MigEnabled bool        `yaml:"mig-enabled"`
	MigDevices []MigDevice `yaml:"mig-devices"`
}

type MigDevice struct {
	Name     string `yaml:"name"`
	Position int    `yaml:"position"`
	Size     int    `yaml:"size"`
}

// A copy of github.com/run-ai/runai-operator/mig-provisioner/pkg/node.MigMapping
type MigMapping map[int][]MigDeviceMappingInfo

type MigDeviceMappingInfo struct {
	Position      int    `json:"position"`
	DeviceUUID    string `json:"device_uuid"`
	GpuInstanceId int    `json:"gpu_instance_id"`
}
