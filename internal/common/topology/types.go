package topology

import "fmt"

// Types
type ClusterTopology struct {
	MigStrategy string                  `yaml:"mig-strategy"`
	Nodes       map[string]NodeTopology `yaml:"nodes"`
}

type NodeTopology struct {
	GpuMemory  int          `yaml:"gpu-memory"`
	GpuProduct string       `yaml:"gpu-product"`
	Gpus       []GpuDetails `yaml:"gpus"`
}

type GpuDetails struct {
	ID      string     `yaml:"id"`
	Metrics GpuMetrics `yaml:"metrics"`
}

type GpuMetrics struct {
	Metadata struct {
		Namespace string `yaml:"namespace"`
		Pod       string `yaml:"pod"`
		Container string `yaml:"container"`
	} `yaml:"metadata"`
	GpuStatus
}
type GpuStatus struct {
	Utilization int `yaml:"utilization"`
	FbUsed      int `yaml:"fb-used"`
}

// Errors
var ErrNoNodes = fmt.Errorf("no nodes found")
var ErrNoNode = fmt.Errorf("node not found")
