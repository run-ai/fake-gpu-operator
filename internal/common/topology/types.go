package topology

import (
	"k8s.io/apimachinery/pkg/types"
)

type ClusterTopology struct {
	NodePools        map[string]NodePoolTopology `yaml:"nodePools"`
	NodePoolLabelKey string                      `yaml:"nodePoolLabelKey"`

	MigStrategy string `yaml:"migStrategy"`
}

type NodePoolTopology struct {
	GpuCount     int             `yaml:"gpuCount"`
	GpuMemory    int             `yaml:"gpuMemory"`
	GpuProduct   string          `yaml:"gpuProduct"`
	OtherDevices []GenericDevice `yaml:"otherDevices"`
}

// ClusterConfig is the new top-level config structure (cluster: key in topology CM).
// Used alongside ClusterTopology during migration; callers switch in Phase 3.
type ClusterConfig struct {
	NodePoolLabelKey string                    `yaml:"nodePoolLabelKey"`
	MigStrategy      string                    `yaml:"migStrategy"`
	NodePools        map[string]NodePoolConfig  `yaml:"nodePools"`
	GpuOperator      *GpuOperatorConfig         `yaml:"gpuOperator,omitempty"`
}

type NodePoolConfig struct {
	Gpu       GpuConfig          `yaml:"gpu"`
	Resources []map[string]int   `yaml:"resources,omitempty"`
}

type GpuConfig struct {
	Backend   string                 `yaml:"backend"`
	Profile   string                 `yaml:"profile,omitempty"`
	Overrides map[string]interface{} `yaml:"overrides,omitempty"`
}

type GpuOperatorConfig struct {
	Version string                 `yaml:"version,omitempty"`
	Values  map[string]interface{} `yaml:"values,omitempty"`
}

type NodeTopology struct {
	GpuMemory    int             `yaml:"gpuMemory"`
	GpuProduct   string          `yaml:"gpuProduct"`
	Gpus         []GpuDetails    `yaml:"gpus"`
	MigStrategy  string          `yaml:"migStrategy"`
	OtherDevices []GenericDevice `yaml:"otherDevices,omitempty"`
}

type GpuDetails struct {
	ID     string    `yaml:"id"`
	Status GpuStatus `yaml:"status"`
}

type PodGpuUsageStatusMap map[types.UID]GpuUsageStatus

type GpuStatus struct {
	AllocatedBy ContainerDetails `yaml:"allocatedBy"`
	// Maps PodUID to its GPU usage status
	PodGpuUsageStatus PodGpuUsageStatusMap `yaml:"podGpuUsageStatus"`
}

type ContainerDetails struct {
	Namespace string `yaml:"namespace"`
	Pod       string `yaml:"pod"`
	Container string `yaml:"container"`
}

type GpuUsageStatus struct {
	Utilization           Range `yaml:"utilization"`
	FbUsed                int   `yaml:"fbUsed"`
	UseKnativeUtilization bool  `yaml:"useKnativeUtilization"`
}

type Range struct {
	Min int `yaml:"min"`
	Max int `yaml:"max"`
}

type GenericDevice struct {
	Name  string `yaml:"name"`
	Count int    `yaml:"count"`
}
