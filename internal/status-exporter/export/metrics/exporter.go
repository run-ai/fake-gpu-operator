package metrics

import (
	"crypto/sha1"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"
)

type MetricsExporter struct {
	topologyChan <-chan *topology.ClusterTopology
}

var _ export.Interface = &MetricsExporter{}

func NewMetricsExporter(watcher watch.Interface) *MetricsExporter {
	topologyChan := make(chan *topology.ClusterTopology)
	watcher.Subscribe(topologyChan)

	return &MetricsExporter{
		topologyChan: topologyChan,
	}
}

func (e *MetricsExporter) Run(stopCh <-chan struct{}) {
	go setupServer()

	// Republish the metrics every 10 seconds to refresh utilization ranges
	// TODO: make this configurable?
	ticker := time.NewTicker(time.Second * 10)
	var clusterTopologyCache *topology.ClusterTopology

	for {
		select {
		case clusterTopology := <-e.topologyChan:
			e.export(clusterTopology)
			clusterTopologyCache = clusterTopology
		case <-ticker.C:
			if clusterTopologyCache != nil {
				e.export(clusterTopologyCache)
			}
		case <-stopCh:
			return
		}
	}
}

func (e *MetricsExporter) export(clusterTopology *topology.ClusterTopology) {
	nodeName := os.Getenv("NODE_NAME")
	node, ok := clusterTopology.Nodes[nodeName]
	if !ok {
		panic(fmt.Sprintf("node %s not found", nodeName))
	}

	gpuUtilization.Reset()
	gpuFbUsed.Reset()
	gpuFbFree.Reset()

	for gpuIdx, gpu := range node.Gpus {
		log.Printf("Exporting metrics for node %v, gpu %v\n", nodeName, gpu.ID)
		labels := prometheus.Labels{
			"gpu":       strconv.Itoa(gpuIdx),
			"UUID":      gpu.ID,
			"device":    "nvidia" + strconv.Itoa(gpuIdx),
			"modelName": node.GpuProduct,
			"Hostname":  generateFakeHostname(nodeName),
			"namespace": gpu.Metrics.Metadata.Namespace,
			"pod":       gpu.Metrics.Metadata.Pod,
			"container": gpu.Metrics.Metadata.Container,
		}

		utilization := gpu.Metrics.PodGpuUsageStatus.Utilization()
		log.Printf("GUY GPU utilization: %v\n", utilization)
		fbUsed := gpu.Metrics.PodGpuUsageStatus.FbUsed(node.GpuMemory)

		gpuUtilization.With(labels).Set(float64(utilization))
		gpuFbUsed.With(labels).Set(float64(fbUsed))
		gpuFbFree.With(labels).Set(float64(node.GpuMemory - fbUsed))
	}
}

func setupServer() {
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe(":9400", nil)
	if err != nil {
		log.Fatal(err)
	}
}

func generateFakeHostname(nodeName string) string {
	h := sha1.New()
	h.Write([]byte(nodeName))
	nodeNameSHA1 := h.Sum(nil)
	nodeHostname := fmt.Sprintf("%s-%x", "nvidia-dcgm-exporter", nodeNameSHA1[:3])
	return nodeHostname
}
