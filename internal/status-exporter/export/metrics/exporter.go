package metrics

import (
	"crypto/sha1"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"
	"github.com/spf13/viper"
)

type MetricsExporter struct {
	topologyChan <-chan *topology.Cluster
}

var _ export.Interface = &MetricsExporter{}

func NewMetricsExporter(watcher watch.Interface) *MetricsExporter {
	topologyChan := make(chan *topology.Cluster)
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
	var clusterTopologyCache *topology.Cluster

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

func (e *MetricsExporter) export(clusterTopology *topology.Cluster) error {
	nodeName := viper.GetString("NODE_NAME")
	node, ok := clusterTopology.Nodes[nodeName]
	if !ok {
		return fmt.Errorf("node %s not found on topology", nodeName)
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
			"namespace": gpu.Status.AllocatedBy.Namespace,
			"pod":       gpu.Status.AllocatedBy.Pod,
			"container": gpu.Status.AllocatedBy.Container,
		}

		utilization := gpu.Status.PodGpuUsageStatus.Utilization()
		fbUsed := gpu.Status.PodGpuUsageStatus.FbUsed(node.GpuMemory)

		gpuUtilization.With(labels).Set(float64(utilization))
		gpuFbUsed.With(labels).Set(float64(fbUsed))
		gpuFbFree.With(labels).Set(float64(node.GpuMemory - fbUsed))
	}

	return nil
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
