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
	topologyChan <-chan *topology.Node
}

var _ export.Interface = &MetricsExporter{}

func NewMetricsExporter(watcher watch.Interface) *MetricsExporter {
	topologyChan := make(chan *topology.Node)
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
	var nodeTopologyCache *topology.Node

	for {
		select {
		case nodeTopology := <-e.topologyChan:
			err := e.export(nodeTopology)
			if err != nil {
				log.Printf("Failed to export metrics: %v", err)
			}
			nodeTopologyCache = nodeTopology
		case <-ticker.C:
			if nodeTopologyCache != nil {
				err := e.export(nodeTopologyCache)
				if err != nil {
					log.Printf("Failed to export metrics: %v", err)
				}
			}
		case <-stopCh:
			return
		}
	}
}

func (e *MetricsExporter) export(nodeTopology *topology.Node) error {
	nodeName := viper.GetString("NODE_NAME")

	gpuUtilization.Reset()
	gpuFbUsed.Reset()
	gpuFbFree.Reset()

	for gpuIdx, gpu := range nodeTopology.Gpus {
		log.Printf("Exporting metrics for node %v, gpu %v\n", nodeName, gpu.ID)
		labels := prometheus.Labels{
			"gpu":       strconv.Itoa(gpuIdx),
			"UUID":      gpu.ID,
			"device":    "nvidia" + strconv.Itoa(gpuIdx),
			"modelName": nodeTopology.GpuProduct,
			"Hostname":  generateFakeHostname(nodeName),
			"namespace": gpu.Status.AllocatedBy.Namespace,
			"pod":       gpu.Status.AllocatedBy.Pod,
			"container": gpu.Status.AllocatedBy.Container,
		}

		utilization := gpu.Status.PodGpuUsageStatus.Utilization()
		fbUsed := gpu.Status.PodGpuUsageStatus.FbUsed(nodeTopology.GpuMemory)

		gpuUtilization.With(labels).Set(float64(utilization))
		gpuFbUsed.With(labels).Set(float64(fbUsed))
		gpuFbFree.With(labels).Set(float64(nodeTopology.GpuMemory - fbUsed))
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
