package labels

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"
	"github.com/spf13/viper"
)

type LabelsExporter struct {
	topologyChan <-chan *topology.Cluster
	kubeclient   kubeclient.KubeClientInterface
	wg           *sync.WaitGroup
}

var _ export.Interface = &LabelsExporter{}

func NewLabelsExporter(watcher watch.Interface, kubeclient kubeclient.KubeClientInterface, wg *sync.WaitGroup) *LabelsExporter {
	topologyChan := make(chan *topology.Cluster)
	watcher.Subscribe(topologyChan)

	return &LabelsExporter{
		topologyChan: topologyChan,
		kubeclient:   kubeclient,
		wg:           wg,
	}
}

func (e *LabelsExporter) Run(stopCh <-chan struct{}) {
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

func (e *LabelsExporter) export(clusterTopology *topology.Cluster) {
	nodeName := viper.GetString("NODE_NAME")
	node, ok := clusterTopology.Nodes[nodeName]
	if !ok {
		panic(fmt.Sprintf("node %s not found", nodeName))
	}

	labels := map[string]string{
		"nvidia.com/gpu.memory":   strconv.Itoa(node.GpuMemory),
		"nvidia.com/gpu.product":  node.GpuProduct,
		"nvidia.com/mig.strategy": clusterTopology.MigStrategy,
		"nvidia.com/gpu.count":    strconv.Itoa(len(node.Gpus)),
	}

	err := e.kubeclient.SetNodeLabels(labels)
	if err != nil {
		panic(err)
	}
}
