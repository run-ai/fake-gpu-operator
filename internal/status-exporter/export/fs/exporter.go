package fs

import (
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/fs/fake"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/fs/real"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"

	"github.com/spf13/viper"
)

func NewFsExporter(watcher watch.Interface) export.Interface {
	topologyChan := make(chan *topology.NodeTopology)
	watcher.Subscribe(topologyChan)

	if viper.GetBool(constants.EnvFakeNode) {
		return fake.NewFakeFsExporter(topologyChan)
	}

	return real.NewRealFsExporter(topologyChan)
}
