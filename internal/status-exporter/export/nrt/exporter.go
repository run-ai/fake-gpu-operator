package nrt

import (
	"context"
	"log"

	"github.com/spf13/viper"
	"k8s.io/client-go/kubernetes"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"
)

// Exporter publishes this node's NodeResourceTopology whenever its topology
// ConfigMap changes (single-node / DaemonSet variant).
type Exporter struct {
	topologyChan <-chan *topology.NodeTopology
	kubeClient   kubernetes.Interface
	publisher    Publisher
}

var _ export.Interface = &Exporter{}

func NewExporter(watcher watch.Interface, kubeClient kubernetes.Interface, publisher Publisher) *Exporter {
	topologyChan := make(chan *topology.NodeTopology)
	watcher.Subscribe(topologyChan)
	return &Exporter{topologyChan: topologyChan, kubeClient: kubeClient, publisher: publisher}
}

func (e *Exporter) Run(stopCh <-chan struct{}) {
	for {
		select {
		case nodeTopology := <-e.topologyChan:
			nodeName := viper.GetString(constants.EnvNodeName)
			if err := publishNodeNRT(context.TODO(), e.kubeClient, e.publisher, nodeName, nodeTopology); err != nil {
				log.Printf("Failed to publish NodeResourceTopology for %s: %v", nodeName, err)
			}
		case <-stopCh:
			return
		}
	}
}
