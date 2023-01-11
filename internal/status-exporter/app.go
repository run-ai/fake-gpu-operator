package status_exporter

import (
	"sync"

	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/fs"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/labels"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/metrics"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"
)

type StatusExporterAppConfig struct {
	NodeName                  string `mapstructure:"NODE_NAME" validator:"required"`
	TopologyCmName            string `mapstructure:"TOPOLOGY_CM_NAME" validator:"required"`
	TopologyCmNamespace       string `mapstructure:"TOPOLOGY_CM_NAMESPACE" validator:"required"`
	TopologyMaxExportInterval string `mapstructure:"TOPOLOGY_MAX_EXPORT_INTERVAL"`
}

type StatusExporterApp struct {
	Watcher        watch.Interface
	MetricExporter export.Interface
	LabelsExporter export.Interface
	FsExporter     export.Interface
	Kubeclient     *kubeclient.KubeClient
	stopCh         chan struct{}
	wg             *sync.WaitGroup
}

func (app *StatusExporterApp) Run() {
	app.wg.Add(4)
	go app.Watcher.Watch(app.stopCh)
	go app.MetricExporter.Run(app.stopCh)
	go app.LabelsExporter.Run(app.stopCh)
	go app.FsExporter.Run(app.stopCh)
}

func (app *StatusExporterApp) Init(stop chan struct{}, wg *sync.WaitGroup) {
	if app.Kubeclient == nil {
		app.Kubeclient = kubeclient.NewKubeClient(nil, stop)
	}
	app.wg = wg

	app.Watcher = watch.NewKubeWatcher(app.Kubeclient, app.wg)
	app.MetricExporter = metrics.NewMetricsExporter(app.Watcher, app.wg)
	app.LabelsExporter = labels.NewLabelsExporter(app.Watcher, app.Kubeclient, app.wg)
	app.FsExporter = fs.NewFsExporter(app.Watcher, app.wg)
}

func (app *StatusExporterApp) Name() string {
	return "StatusExporter"
}

func (app *StatusExporterApp) GetConfig() interface{} {
	var config StatusExporterAppConfig
	return config
}
