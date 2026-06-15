package nrt

import (
	"sync"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

type fakeWatcher struct{ ch chan<- *topology.NodeTopology }

func (w *fakeWatcher) Subscribe(subscriber chan<- *topology.NodeTopology) { w.ch = subscriber }
func (w *fakeWatcher) Watch(stopCh <-chan struct{})                       {}

func TestExporter_PublishesOnTopologyEvent(t *testing.T) {
	viper.Set(constants.EnvNodeName, "node-a")
	client := seedCluster(t, numaClusterYAML, gpuNode("node-a", "default"))
	pub := &recordingPublisher{}
	fw := &fakeWatcher{}

	exporter := NewExporter(fw, client, pub)

	stop := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() { defer wg.Done(); exporter.Run(stop) }()

	fw.ch <- topo(8)
	stop <- struct{}{}
	wg.Wait()

	require.Len(t, pub.applied, 1)
	assert.Equal(t, "node-a", pub.applied[0].Name)
}
