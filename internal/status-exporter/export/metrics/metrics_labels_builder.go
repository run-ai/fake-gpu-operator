package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

// buildGpuMetricLabels creates Prometheus labels for a GPU metric
func buildGpuMetricLabels(nodeName string, gpuIdx int, gpu *topology.GpuDetails, nodeTopology *topology.NodeTopology) prometheus.Labels {
	return prometheus.Labels{
		"gpu":       strconv.Itoa(gpuIdx),
		"UUID":      gpu.ID,
		"device":    "nvidia" + strconv.Itoa(gpuIdx),
		"modelName": nodeTopology.GpuProduct,
		"Hostname":  generateFakeHostname(nodeName),
		"namespace": gpu.Status.AllocatedBy.Namespace,
		"pod":       gpu.Status.AllocatedBy.Pod,
		"container": gpu.Status.AllocatedBy.Container,
	}
}
