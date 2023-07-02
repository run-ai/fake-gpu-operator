package topology

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
)

type BaseTopology struct {
	MigStrategy string `json:"mig-strategy"`
	Config      Config `json:"config"`
}

type NodeTopology struct {
	GpuMemory   int          `json:"gpu-memory"`
	GpuProduct  string       `json:"gpu-product"`
	Gpus        []GpuDetails `json:"gpus"`
	MigStrategy string       `json:"mig-strategy"`
}

type GpuDetails struct {
	ID     string    `json:"id"`
	Status GpuStatus `json:"status"`
}

type PodGpuUsageStatusMap map[types.UID]GpuUsageStatus

func (m PodGpuUsageStatusMap) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("{}"), nil
	}

	return json.Marshal(map[types.UID]GpuUsageStatus(m))
}

type GpuStatus struct {
	AllocatedBy ContainerDetails `json:"allocated-by"`
	// Maps PodUID to its GPU usage status
	PodGpuUsageStatus PodGpuUsageStatusMap `json:"pod-gpu-usage-status"`
}

type ContainerDetails struct {
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
}

type GpuUsageStatus struct {
	Utilization           Range `json:"utilization"`
	FbUsed                int   `json:"fb-used"`
	UseKnativeUtilization bool  `json:"use-knative-utilization"`
}

type Range struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type Config struct {
	NodeAutofill NodeAutofillSettings `json:"node-autofill"`
}

type NodeAutofillSettings struct {
	GpuCount   int    `json:"gpu-count"`
	GpuMemory  int    `json:"gpu-memory"`
	GpuProduct string `json:"gpu-product"`
}

// Errors
var ErrNoNodes = fmt.Errorf("no nodes found")
var ErrNoNode = fmt.Errorf("node not found")
