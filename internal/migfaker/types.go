package migfaker

type AnnotationMigConfig struct {
	Version    string     `yaml:"version"`
	MigConfigs MigConfigs `yaml:"mig-configs"`
}

type MigConfigs struct {
	SelectedDevices []SelectedDevices `yaml:"selected"`
}

type SelectedDevices struct {
	Devices    []string          `yaml:"devices"`
	MigEnabled bool              `yaml:"mig-enabled"`
	MigDevices map[string]string `yaml:"mig-devices"`
}
