package fs

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"
	"github.com/spf13/viper"
)

type FsExporter struct {
	topologyChan <-chan *topology.Cluster
	wg           *sync.WaitGroup
}

var _ export.Interface = &FsExporter{}

func NewFsExporter(watcher watch.Interface, wg *sync.WaitGroup) *FsExporter {
	topologyChan := make(chan *topology.Cluster)
	watcher.Subscribe(topologyChan)

	return &FsExporter{
		topologyChan: topologyChan,
		wg:           wg,
	}
}

func (e *FsExporter) Run(stopCh <-chan struct{}) {
	defer e.wg.Done()
	for {
		select {
		case clusterTopology := <-e.topologyChan:
			e.export(clusterTopology)
		case <-stopCh:
			return
		}
	}
}

func (e *FsExporter) export(clusterTopology *topology.Cluster) {
	nodeName := viper.GetString("NODE_NAME")
	node, ok := clusterTopology.Nodes[nodeName]
	if !ok {
		panic(fmt.Sprintf("node %s not found", nodeName))
	}

	for gpuIdx, gpu := range node.Gpus {
		// Ignoring pods that are not supposed to be seen by runai-container-toolkit
		if gpu.Status.AllocatedBy.Namespace != "runai-reservation" {
			continue
		}

		for podUuid, gpuUsageStatus := range gpu.Status.PodGpuUsageStatus {
			log.Printf("Exporting pod %s gpu utilization to filesystem", podUuid)
			utilization := gpuUsageStatus.Utilization.Random()

			path := fmt.Sprintf("runai/proc/pod/%s/metrics/gpu/%d/utilization.sm", podUuid, gpuIdx)
			if err := os.MkdirAll(filepath.Dir(path), 0644); err != nil {
				log.Printf("Failed creating directory for pod %s: %s", podUuid, err.Error())
			}

			if err := os.WriteFile(path, []byte(strconv.Itoa(utilization)), 0644); err != nil {
				log.Printf("Failed exporting pod %s to filesystem: %s", podUuid, err.Error())
			}
		}
	}
}
