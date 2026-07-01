package status_exporter

import (
	"sync"

	"github.com/spf13/viper"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/fs"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/labels"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/metrics"
	nrtexport "github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/nrt"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/podresources"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"
)

type StatusExporterAppConfig struct {
	NodeName                  string `mapstructure:"NODE_NAME" validator:"required"`
	TopologyCmName            string `mapstructure:"TOPOLOGY_CM_NAME" validator:"required"`
	TopologyCmNamespace       string `mapstructure:"TOPOLOGY_CM_NAMESPACE" validator:"required"`
	TopologyMaxExportInterval string `mapstructure:"TOPOLOGY_MAX_EXPORT_INTERVAL"`
	PrometheusURL             string `mapstructure:"PROMETHEUS_URL"`
}

type StatusExporterApp struct {
	Watcher              watch.Interface
	MetricExporter       export.Interface
	LabelsExporter       export.Interface
	FsExporter           export.Interface
	NRTExporter          export.Interface
	PodResourcesExporter export.Interface
	Kubeclient           *kubeclient.KubeClient
	stopCh               chan struct{}
	wg                   *sync.WaitGroup
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

	if app.NRTExporter != nil {
		app.wg.Add(1)
		go func() {
			defer app.wg.Done()
			app.NRTExporter.Run(app.stopCh)
		}()
	}

	if app.PodResourcesExporter != nil {
		app.wg.Add(1)
		go func() {
			defer app.wg.Done()
			app.PodResourcesExporter.Run(app.stopCh)
		}()
	}

	app.wg.Wait()
}

func (app *StatusExporterApp) Init(stop chan struct{}) {
	// Initialize Prometheus configuration
	prometheusURL := viper.GetString(constants.EnvPrometheusURL)
	topology.InitPrometheusConfig(prometheusURL)

	if app.Kubeclient == nil {
		app.Kubeclient = kubeclient.NewKubeClient(nil, stop)
	}
	app.wg = &sync.WaitGroup{}

	app.Watcher = watch.NewKubeWatcher(app.Kubeclient)
	app.MetricExporter = metrics.NewMetricsExporter(app.Watcher)
	app.LabelsExporter = labels.NewLabelsExporter(app.Watcher, app.Kubeclient)
	app.FsExporter = fs.NewFsExporter(app.Watcher)

	if viper.GetBool(constants.EnvNodeResourceTopologyEnabled) {
		cfg := ctrl.GetConfigOrDie()
		app.NRTExporter = nrtexport.NewExporter(
			app.Watcher,
			kubernetes.NewForConfigOrDie(cfg),
			nrtexport.NewReconciler(dynamic.NewForConfigOrDie(cfg)),
		)
	}

	if viper.GetBool(constants.EnvPodResourcesEnabled) {
		cfg := ctrl.GetConfigOrDie()
		app.PodResourcesExporter = podresources.NewExporter(app.Watcher, kubernetes.NewForConfigOrDie(cfg))
	}
}

func (app *StatusExporterApp) Name() string {
	return "StatusExporter"
}

func (app *StatusExporterApp) GetConfig() interface{} {
	var config StatusExporterAppConfig
	return config
}
