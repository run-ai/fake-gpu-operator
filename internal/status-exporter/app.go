package status_exporter

import (
	"sync"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/fs"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/labels"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/metrics"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"
	"github.com/spf13/viper"
)

type StatusExporterAppConfig struct {
	NodeName                  string `mapstructure:"NODE_NAME" validator:"required"`
	TopologyCmName            string `mapstructure:"TOPOLOGY_CM_NAME" validator:"required"`
	TopologyCmNamespace       string `mapstructure:"TOPOLOGY_CM_NAMESPACE" validator:"required"`
	TopologyMaxExportInterval string `mapstructure:"TOPOLOGY_MAX_EXPORT_INTERVAL"`
	PrometheusURL             string `mapstructure:"PROMETHEUS_URL"`
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

	go func() {
		defer app.wg.Done()
		app.Watcher.Watch(app.stopCh)
	}()

	go func() {
		defer app.wg.Done()
		app.MetricExporter.Run(app.stopCh)
	}()

	go func() {
		defer app.wg.Done()
		app.LabelsExporter.Run(app.stopCh)
	}()

	go func() {
		defer app.wg.Done()
		app.FsExporter.Run(app.stopCh)
	}()

	app.wg.Wait()
}

func (app *StatusExporterApp) Init(stop chan struct{}) {
	// Initialize Prometheus configuration
	prometheusURL := viper.GetString(constants.EnvPrometheusURL)
	if prometheusURL == "" {
		prometheusURL = "http://prometheus-operated.runai:9090"
	}
	topology.InitPrometheusConfig(prometheusURL)

	if app.Kubeclient == nil {
		app.Kubeclient = kubeclient.NewKubeClient(nil, stop)
	}
	app.wg = &sync.WaitGroup{}

	app.Watcher = watch.NewKubeWatcher(app.Kubeclient)
	app.MetricExporter = metrics.NewMetricsExporter(app.Watcher)
	app.LabelsExporter = labels.NewLabelsExporter(app.Watcher, app.Kubeclient)
	app.FsExporter = fs.NewFsExporter(app.Watcher)
}

func (app *StatusExporterApp) Name() string {
	return "StatusExporter"
}

func (app *StatusExporterApp) GetConfig() interface{} {
	var config StatusExporterAppConfig
	return config
}
