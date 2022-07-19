package topology

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
)

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

type PodGpuUsageStatusMap map[types.UID]GpuUsageStatus

type GpuMetrics struct {
	Metadata GpuMetricsMetadata `yaml:"metadata"`
	// Maps PodUID to its GPU usage status
	PodGpuUsageStatus PodGpuUsageStatusMap `yaml:"podGpuUsageStatus"`
}

type GpuMetricsMetadata struct {
	Namespace string `yaml:"namespace"`
	Pod       string `yaml:"pod"`
	Container string `yaml:"container"`
}

type GpuUsageStatus struct {
	Utilization    Range `yaml:"utilization"`
	FbUsed         int   `yaml:"fb-used"`
	IsInferencePod bool  `yaml:"is-inference-pod"`
}

type Range struct {
	Min int `yaml:"min"`
	Max int `yaml:"max"`
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
