package fs

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"
)

type FsExporter struct {
	topologyChan <-chan *topology.NodeTopology
}

var _ export.Interface = &FsExporter{}

func NewFsExporter(watcher watch.Interface) *FsExporter {
	topologyChan := make(chan *topology.NodeTopology)
	watcher.Subscribe(topologyChan)

	return &FsExporter{
		topologyChan: topologyChan,
	}
}

func (e *FsExporter) Run(stopCh <-chan struct{}) {
	for {
		select {
		case nodeTopology := <-e.topologyChan:
			e.export(nodeTopology)
		case <-stopCh:
			return
		}
	}
}

func (e *FsExporter) export(nodeTopology *topology.NodeTopology) {
	if err := os.RemoveAll("/runai/proc/pod"); err != nil {
		log.Printf("Failed deleting runai/proc/pod directory: %s", err.Error())
	}

	for gpuIdx, gpu := range nodeTopology.Gpus {
		// Ignoring pods that are not supposed to be seen by runai-container-toolkit
		if gpu.Status.AllocatedBy.Namespace != constants.ReservationNs {
			continue
		}

		for podUuid, gpuUsageStatus := range gpu.Status.PodGpuUsageStatus {
			log.Printf("Exporting pod %s gpu stats to filesystem", podUuid)

			path := fmt.Sprintf("/runai/proc/pod/%s/metrics/gpu/%d", podUuid, gpuIdx)
			if err := os.MkdirAll(path, 0755); err != nil {
				log.Printf("Failed creating directory for pod %s: %s", podUuid, err.Error())
			}

			if err := writeFile(filepath.Join(path, "utilization.sm"), []byte(strconv.Itoa(gpuUsageStatus.Utilization.Random()))); err != nil {
				log.Printf("Failed exporting utilization for pod %s: %s", podUuid, err.Error())
			}

			if err := writeFile(filepath.Join(path, "memory.allocated"), []byte(strconv.Itoa(mbToBytes(gpuUsageStatus.FbUsed)))); err != nil {
				log.Printf("Failed exporting memory for pod %s: %s", podUuid, err.Error())
			}
		}
	}
}

func writeFile(path string, content []byte) error {
	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("failed writing file %s: %w", path, err)
	}
	return nil
}

func mbToBytes(mb int) int {
	return mb * (1000 * 1000)
}
