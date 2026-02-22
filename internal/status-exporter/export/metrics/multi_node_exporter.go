package metrics

import (
	"log"
	"sync"
	"time"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"
)

// MultiNodeMetricsExporter exports metrics for multiple KWOK nodes
type MultiNodeMetricsExporter struct {
	nodeTopologies map[string]*topology.NodeTopology
	mu             sync.RWMutex
	stopCh         chan struct{}
}

var _ watch.MetricsExporter = &MultiNodeMetricsExporter{}

// NewMultiNodeMetricsExporter creates a new multi-node metrics exporter
func NewMultiNodeMetricsExporter() *MultiNodeMetricsExporter {
	return &MultiNodeMetricsExporter{
		nodeTopologies: make(map[string]*topology.NodeTopology),
		stopCh:         make(chan struct{}),
	}
}

// SetMetricsForNode exports metrics for a specific node
func (e *MultiNodeMetricsExporter) SetMetricsForNode(nodeName string, nodeTopology *topology.NodeTopology) error {
	// Update cache
	e.mu.Lock()
	e.nodeTopologies[nodeName] = nodeTopology
	e.mu.Unlock()

	// Export immediately
	return e.exportNode(nodeName, nodeTopology)
}

// DeleteNode removes metrics for a deleted node
func (e *MultiNodeMetricsExporter) DeleteNode(nodeName string) error {
	e.mu.Lock()
	nodeTopology, exists := e.nodeTopologies[nodeName]
	delete(e.nodeTopologies, nodeName)
	e.mu.Unlock()

	if !exists {
		return nil
	}

	// Delete metrics for this node's GPUs
	log.Printf("Deleting metrics for KWOK node: %s\n", nodeName)
	e.deleteNodeMetrics(nodeName, nodeTopology)
	return nil
}

// Run starts the metrics refresh ticker
func (e *MultiNodeMetricsExporter) Run(stopCh <-chan struct{}) {
	go setupServer()

	// Republish the metrics every 10 seconds to refresh utilization ranges
	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.refreshAllMetrics()
		case <-stopCh:
			close(e.stopCh)
			return
		}
	}
}

// refreshAllMetrics re-exports metrics for all known nodes
func (e *MultiNodeMetricsExporter) refreshAllMetrics() {
	// Copy map while holding lock (fast operation)
	e.mu.RLock()
	snapshot := make(map[string]*topology.NodeTopology, len(e.nodeTopologies))
	for nodeName, nodeTopology := range e.nodeTopologies {
		snapshot[nodeName] = nodeTopology
	}
	e.mu.RUnlock()

	for nodeName, nodeTopology := range snapshot {
		if err := e.exportNode(nodeName, nodeTopology); err != nil {
			log.Printf("Failed to refresh metrics for node %s: %v\n", nodeName, err)
		}
	}
}

// exportNode exports metrics for a single node
func (e *MultiNodeMetricsExporter) exportNode(nodeName string, nodeTopology *topology.NodeTopology) error {
	for gpuIdx, gpu := range nodeTopology.Gpus {
		log.Printf("Exporting metrics for KWOK node %s, gpu %s\n", nodeName, gpu.ID)
		labels := buildGpuMetricLabels(nodeName, gpuIdx, &gpu, nodeTopology)

		// Override Hostname with the node name for KWOK nodes so that the
		// metrics-exporter can match metrics to the correct virtual node.
		// The default generateFakeHostname produces a hash that cannot be
		// correlated back to the KWOK node name.
		labels["Hostname"] = nodeName

		utilization := gpu.Status.PodGpuUsageStatus.Utilization()
		fbUsed := gpu.Status.PodGpuUsageStatus.FbUsed(nodeTopology.GpuMemory)

		gpuUtilization.With(labels).Set(float64(utilization))
		gpuFbUsed.With(labels).Set(float64(fbUsed))
		gpuFbFree.With(labels).Set(float64(nodeTopology.GpuMemory - fbUsed))
	}

	return nil
}

// deleteNodeMetrics deletes Prometheus metrics for a specific node
func (e *MultiNodeMetricsExporter) deleteNodeMetrics(nodeName string, nodeTopology *topology.NodeTopology) {
	for gpuIdx, gpu := range nodeTopology.Gpus {
		labels := buildGpuMetricLabels(nodeName, gpuIdx, &gpu, nodeTopology)
		labels["Hostname"] = nodeName

		// Delete the metric series for this GPU
		gpuUtilization.Delete(labels)
		gpuFbUsed.Delete(labels)
		gpuFbFree.Delete(labels)
	}
}