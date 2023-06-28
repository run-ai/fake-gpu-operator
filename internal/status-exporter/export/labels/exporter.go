package labels

import (
	"fmt"
	"log"
	"strconv"

	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"
)

type LabelsExporter struct {
	topologyChan <-chan *topology.Node
	kubeclient   kubeclient.KubeClientInterface
}

var _ export.Interface = &LabelsExporter{}

func NewLabelsExporter(watcher watch.Interface, kubeclient kubeclient.KubeClientInterface) *LabelsExporter {
	topologyChan := make(chan *topology.Node)
	watcher.Subscribe(topologyChan)

	return &LabelsExporter{
		topologyChan: topologyChan,
		kubeclient:   kubeclient,
	}
}

func (e *LabelsExporter) Run(stopCh <-chan struct{}) {
	for {
		select {
		case nodeTopology := <-e.topologyChan:
			err := e.export(nodeTopology)
			if err != nil {
				log.Printf("Failed to export labels: %v", err)
			}
		case <-stopCh:
			return
		}
	}
}

func (e *LabelsExporter) export(nodeTopology *topology.Node) error {

	labels := map[string]string{
		"nvidia.com/gpu.memory":                       strconv.Itoa(nodeTopology.GpuMemory),
		"nvidia.com/gpu.product":                      nodeTopology.GpuProduct,
		"nvidia.com/mig.strategy":                     nodeTopology.MigStrategy,
		"nvidia.com/gpu.count":                        strconv.Itoa(len(nodeTopology.Gpus)),
		"feature.node.kubernetes.io/pci-10de.present": "true",
	}

	err := e.kubeclient.SetNodeLabels(labels)
	if err != nil {
		return fmt.Errorf("failed to set node labels: %w", err)
	}

	return nil
}
