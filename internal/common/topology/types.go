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

type NodeTopology struct {
	GpuMemory    int             `yaml:"gpuMemory" json:"gpuMemory"`
	GpuProduct   string          `yaml:"gpuProduct" json:"gpuProduct"`
	Gpus         []GpuDetails    `yaml:"gpus" json:"gpus"`
	MigStrategy  string          `yaml:"migStrategy" json:"migStrategy"`
	OtherDevices []GenericDevice `yaml:"otherDevices,omitempty" json:"otherDevices,omitempty"`
}

type GpuDetails struct {
	ID     string    `yaml:"id" json:"id"`
	Status GpuStatus `yaml:"status" json:"status"`
}

type PodGpuUsageStatusMap map[types.UID]GpuUsageStatus

type GpuStatus struct {
	AllocatedBy ContainerDetails `yaml:"allocatedBy" json:"allocatedBy"`
	// Maps PodUID to its GPU usage status
	PodGpuUsageStatus PodGpuUsageStatusMap `yaml:"podGpuUsageStatus" json:"podGpuUsageStatus"`
}

type ContainerDetails struct {
	Namespace string `yaml:"namespace" json:"namespace"`
	Pod       string `yaml:"pod" json:"pod"`
	Container string `yaml:"container" json:"container"`
}

type GpuUsageStatus struct {
	Utilization           Range `yaml:"utilization" json:"utilization"`
	FbUsed                int   `yaml:"fbUsed" json:"fbUsed"`
	UseKnativeUtilization bool  `yaml:"useKnativeUtilization" json:"useKnativeUtilization"`
}

type Range struct {
	Min int `yaml:"min" json:"min"`
	Max int `yaml:"max" json:"max"`
}

type GenericDevice struct {
	Name  string `yaml:"name"`
	Count int    `yaml:"count"`
}
