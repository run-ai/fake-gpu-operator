package status_exporter

import (
	"sync"

	"github.com/run-ai/fake-gpu-operator/internal/common/config"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/fs"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/labels"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/metrics"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var InClusterConfigFn = rest.InClusterConfig
var KubeClientFn = func(c *rest.Config) kubernetes.Interface {
	return kubernetes.NewForConfigOrDie(c)
}

type StatusExporterApp struct {
	Watcher        watch.Interface
	MetricExporter export.Interface
	LabelsExporter export.Interface
	FsExporter     export.Interface
}

func NewStatusExporterApp() *StatusExporterApp {
	return &StatusExporterApp{}
}

func (app *StatusExporterApp) Start(stopper chan struct{}, wg *sync.WaitGroup) {
	app.Init()

	wg.Add(4)
	go app.Watcher.Watch(stopper, wg)
	go app.MetricExporter.Run(stopper, wg)
	go app.LabelsExporter.Run(stopper, wg)
	go app.FsExporter.Run(stopper, wg)
}

func (app *StatusExporterApp) Init() {
	requiredEnvVars := []string{"NODE_NAME", "TOPOLOGY_CM_NAME", "TOPOLOGY_CM_NAMESPACE"}
	config.ValidateConfig(requiredEnvVars)

	config, err := InClusterConfigFn()
	if err != nil {
		panic(err.Error())
	}
	kubeclient := KubeClientFn(config)

	app.Watcher = watch.NewKubeWatcher(kubeclient)
	app.MetricExporter = metrics.NewMetricsExporter(app.Watcher)
	app.LabelsExporter = labels.NewLabelsExporter(app.Watcher, kubeclient)
	app.FsExporter = fs.NewFsExporter(app.Watcher)
}

func (app *StatusExporterApp) Name() string {
	return "StatusExporter"
}
