package labels

import (
	"fmt"
	"log"
	"strconv"

	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"
	"github.com/spf13/viper"
)

type LabelsExporter struct {
	topologyChan <-chan *topology.Cluster
	kubeclient   kubeclient.KubeClientInterface
}

var _ export.Interface = &LabelsExporter{}

func NewLabelsExporter(watcher watch.Interface, kubeclient kubeclient.KubeClientInterface) *LabelsExporter {
	topologyChan := make(chan *topology.Cluster)
	watcher.Subscribe(topologyChan)

	return &LabelsExporter{
		topologyChan: topologyChan,
		kubeclient:   kubeclient,
	}
}

func (e *LabelsExporter) Run(stopCh <-chan struct{}) {
	for {
		select {
		case clusterTopology := <-e.topologyChan:
			err := e.export(clusterTopology)
			if err != nil {
				log.Printf("Failed to export labels: %v", err)
			}
		case <-stopCh:
			return
		}
	}
}

func (e *LabelsExporter) export(clusterTopology *topology.Cluster) error {
	nodeName := viper.GetString("NODE_NAME")
	node, ok := clusterTopology.Nodes[nodeName]
	if !ok {
		return fmt.Errorf("node %s not found", nodeName)
	}

	labels := map[string]string{
		"nvidia.com/gpu.memory":                       strconv.Itoa(node.GpuMemory),
		"nvidia.com/gpu.product":                      node.GpuProduct,
		"nvidia.com/mig.strategy":                     clusterTopology.MigStrategy,
		"nvidia.com/gpu.count":                        strconv.Itoa(len(node.Gpus)),
		"feature.node.kubernetes.io/pci-10de.present": "true",
	}

	err := e.kubeclient.SetNodeLabels(labels)
	if err != nil {
		return fmt.Errorf("failed to set node labels: %v", err)
	}

	return nil
}
