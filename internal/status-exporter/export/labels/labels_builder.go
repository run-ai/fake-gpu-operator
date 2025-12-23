package labels

import (
	"strconv"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

// BuildNodeLabels creates the standard node labels from a topology
func BuildNodeLabels(nodeTopology *topology.NodeTopology) map[string]string {
	return map[string]string{
		"nvidia.com/gpu.memory":   strconv.Itoa(nodeTopology.GpuMemory),
		"nvidia.com/gpu.product":  nodeTopology.GpuProduct,
		"nvidia.com/mig.strategy": nodeTopology.MigStrategy,
		"nvidia.com/gpu.count":    strconv.Itoa(len(nodeTopology.Gpus)),
		"nvidia.com/gpu.present":  "true",
		"run.ai/fake.gpu":         "true",
	}
}
