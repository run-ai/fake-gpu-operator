package labels_test

import (
	"strconv"
	"strings"
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
		assert.Equal(t, labels["nvidia.com/gpu.product"], "some-gpu") // spaces sanitized to dashes
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

func TestBuildNodeLabels_SanitizesGpuProduct(t *testing.T) {
	tests := []struct {
		name       string
		gpuProduct string
		wantLabel  string
	}{
		{
			name:       "spaces replaced with dashes",
			gpuProduct: "NVIDIA A100-SXM4-40GB",
			wantLabel:  "NVIDIA-A100-SXM4-40GB",
		},
		{
			name:       "slashes replaced",
			gpuProduct: "NVIDIA A100/SXM4",
			wantLabel:  "NVIDIA-A100-SXM4",
		},
		{
			name:       "parentheses replaced",
			gpuProduct: "GPU (Test Edition)",
			wantLabel:  "GPU--Test-Edition",
		},
		{
			name:       "truncated to 63 chars",
			gpuProduct: strings.Repeat("A", 100),
			wantLabel:  strings.Repeat("A", 63),
		},
		{
			name:       "empty string",
			gpuProduct: "",
			wantLabel:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topo := &topology.NodeTopology{
				GpuProduct: tt.gpuProduct,
				Gpus:       []topology.GpuDetails{{ID: "gpu-1"}},
			}
			l := labels.BuildNodeLabels(topo)
			assert.Equal(t, tt.wantLabel, l["nvidia.com/gpu.product"])
		})
	}
}
