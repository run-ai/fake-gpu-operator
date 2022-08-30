package status_exporter

import (
	"log"
	"os"
	"os/signal"
	"syscall"

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

type App struct {
	stopper chan struct{}
}

func NewApp() *App {
	app := &App{
		stopper: make(chan struct{}),
	}
	return app
}

func (app *App) Run(readyCh chan<- struct{}) {
	defer app.Stop()

	requiredEnvVars := []string{"NODE_NAME", "TOPOLOGY_CM_NAME", "TOPOLOGY_CM_NAMESPACE"}
	config.ValidateConfig(requiredEnvVars)

	config, err := InClusterConfigFn()
	if err != nil {
		panic(err.Error())
	}
	kubeclient := KubeClientFn(config)

	// Watch for changes, and export metrics
	stopper := make(chan struct{})
	var watcher watch.Interface = watch.NewKubeWatcher(kubeclient)
	var metricExporter export.Interface = metrics.NewMetricsExporter(watcher)
	var labelsExporter export.Interface = labels.NewLabelsExporter(watcher, kubeclient)
	var fsExporter export.Interface = fs.NewFsExporter(watcher)

	go watcher.Watch(stopper, readyCh)
	go metricExporter.Run(stopper)
	go labelsExporter.Run(stopper)
	go fsExporter.Run(stopper)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	s := <-sig
	log.Printf("Received signal \"%v\"\n", s)
}

func (app *App) Stop() {
	close(app.stopper)
}
