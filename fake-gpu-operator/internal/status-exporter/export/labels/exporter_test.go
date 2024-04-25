package labels_test

import (
	"strconv"
	"sync"
	"testing"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/labels"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

type FakeWatcher struct {
	topologyChan chan<- *topology.NodeTopology
}

func (watcher *FakeWatcher) Subscribe(subscriber chan<- *topology.NodeTopology) {
	watcher.topologyChan = subscriber
}
func (watcher *FakeWatcher) Watch(stopCh <-chan struct{}) {}

func TestExport(t *testing.T) {
	viper.SetDefault(constants.EnvNodeName, "my_node")

	myNode := &topology.NodeTopology{
		GpuProduct: "some gpu",
		Gpus: []topology.GpuDetails{
			{
				ID: "stam",
			},
		},
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	kubeClientMock := &kubeclient.KubeClientMock{}
	kubeClientMock.ActualSetNodeLabels = func(labels map[string]string) {
		assert.Equal(t, labels["nvidia.com/gpu.memory"], strconv.Itoa(myNode.GpuMemory))
		assert.Equal(t, labels["nvidia.com/gpu.product"], myNode.GpuProduct)
		assert.Equal(t, labels["nvidia.com/mig.strategy"], myNode.MigStrategy)
		assert.Equal(t, labels["nvidia.com/gpu.count"], strconv.Itoa(len(myNode.Gpus)))
		wg.Done()
	}

	fakeWatcher := &FakeWatcher{}
	lablesExporter := labels.NewLabelsExporter(fakeWatcher, kubeClientMock)
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		lablesExporter.Run(stop)
	}()

	fakeWatcher.topologyChan <- myNode
	stop <- struct{}{}
	wg.Wait()
}
