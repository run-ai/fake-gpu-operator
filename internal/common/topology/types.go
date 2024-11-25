package topology

import (
	"fmt"

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
	OtherDevices []GenericDevice `yaml:"otherDevices,omitempty"`
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

// Errors
var ErrNoNodes = fmt.Errorf("no nodes found")
var ErrNoNode = fmt.Errorf("node not found")
