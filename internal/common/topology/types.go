package topology

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
)

type BaseTopology struct {
	Config Config `json:"config"`
}

type NodeTopology struct {
	GpuMemory   int          `yaml:"gpu-memory"`
	GpuProduct  string       `yaml:"gpu-product"`
	Gpus        []GpuDetails `yaml:"gpus"`
	MigStrategy string       `yaml:"mig-strategy"`
}

type GpuDetails struct {
	ID     string    `json:"id"`
	Status GpuStatus `json:"status"`
}

type PodGpuUsageStatusMap map[types.UID]GpuUsageStatus

type GpuStatus struct {
	AllocatedBy ContainerDetails `yaml:"allocated-by"`
	// Maps PodUID to its GPU usage status
	PodGpuUsageStatus PodGpuUsageStatusMap `yaml:"pod-gpu-usage-status"`
}

type ContainerDetails struct {
	Namespace string `yaml:"namespace"`
	Pod       string `yaml:"pod"`
	Container string `yaml:"container"`
}

type GpuUsageStatus struct {
	Utilization           Range `yaml:"utilization"`
	FbUsed                int   `yaml:"fb-used"`
	UseKnativeUtilization bool  `yaml:"use-knative-utilization"`
}

type Range struct {
	Min int `yaml:"min"`
	Max int `yaml:"max"`
}

type Config struct {
	NodeAutofill     NodeAutofillSettings `yaml:"node-autofill"`
	FakeNodeHandling bool                 `yaml:"fake-node-handling"`
}

type NodeAutofillSettings struct {
	GpuCount    int    `yaml:"gpu-count"`
	GpuMemory   int    `yaml:"gpu-memory"`
	GpuProduct  string `yaml:"gpu-product"`
	MigStrategy string `yaml:"mig-strategy"`
}

// Errors
var ErrNoNodes = fmt.Errorf("no nodes found")
var ErrNoNode = fmt.Errorf("node not found")
