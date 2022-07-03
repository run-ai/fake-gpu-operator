package topology

import "fmt"

// Types
type ClusterTopology struct {
	MigStrategy string                  `yaml:"mig-strategy"`
	Nodes       map[string]NodeTopology `yaml:"nodes"`
	Config      Config                  `yaml:"config"`
}

type NodeTopology struct {
	GpuCount   int          `yaml:"gpu-count"`
	GpuMemory  int          `yaml:"gpu-memory"`
	GpuProduct string       `yaml:"gpu-product"`
	Gpus       []GpuDetails `yaml:"gpus"`
}

type GpuDetails struct {
	ID      string     `yaml:"id"`
	Metrics GpuMetrics `yaml:"metrics"`
}

type GpuMetrics struct {
	Metadata GpuMetricsMetadata `yaml:"metadata"`
	Status   GpuStatus          `yaml:"status"`
}

type GpuMetricsMetadata struct {
	Namespace string `yaml:"namespace"`
	Pod       string `yaml:"pod"`
	Container string `yaml:"container"`
}

type GpuStatus struct {
	Utilization int `yaml:"utilization"`
	FbUsed      int `yaml:"fb-used"`
}

type Config struct {
	NodeAutofill NodeAutofillSettings `yaml:"node-autofill"`
}

type NodeAutofillSettings struct {
	Enabled    bool   `yaml:"enabled"`
	GpuCount   int    `yaml:"gpu-count"`
	GpuMemory  int    `yaml:"gpu-memory"`
	GpuProduct string `yaml:"gpu-product"`
}

// Errors
var ErrNoNodes = fmt.Errorf("no nodes found")
var ErrNoNode = fmt.Errorf("node not found")
