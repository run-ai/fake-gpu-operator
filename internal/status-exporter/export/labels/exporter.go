package labels

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"
	"github.com/spf13/viper"
	"k8s.io/client-go/kubernetes"
)

type LabelsExporter struct {
	topologyChan <-chan *topology.ClusterTopology
	kubeclient   kubernetes.Interface
}

var _ export.Interface = &LabelsExporter{}

func NewLabelsExporter(watcher watch.Interface, kubeclient kubernetes.Interface) *LabelsExporter {
	topologyChan := make(chan *topology.ClusterTopology)
	watcher.Subscribe(topologyChan)

	return &LabelsExporter{
		topologyChan: topologyChan,
		kubeclient:   kubeclient,
	}
}

func (e *LabelsExporter) Run(stopCh <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case clusterTopology := <-e.topologyChan:
			e.export(clusterTopology)
		case <-stopCh:
			return
		}
	}
}

func (e *LabelsExporter) export(clusterTopology *topology.ClusterTopology) {
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

	err := e.labelNode(nodeName, labels)
	if err != nil {
		panic(err)
	}
}

func (e *LabelsExporter) labelNode(nodeName string, labels map[string]string) error {
	node, err := e.kubeclient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	for k, v := range labels {
		node.Labels[k] = v
	}

	log.Printf("labelling node %s with %v\n", nodeName, labels)
	_, err = e.kubeclient.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
	return err
}
