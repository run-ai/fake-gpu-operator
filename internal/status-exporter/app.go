package status_exporter

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/run-ai/fake-gpu-operator/internal/common/config"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export"
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

func Run() {
	requiredEnvVars := []string{"NODE_NAME", "TOPOLOGY_CM_NAME", "TOPOLOGY_CM_NAMESPACE"}
	config.ValidateConfig(requiredEnvVars)

	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	kubeclient := kubernetes.NewForConfigOrDie(config)

	// Watch for changes, and export metrics
	stopper := make(chan struct{})
	var watcher watch.Interface = watch.NewKubeWatcher(kubeclient)
	var metricExporter export.Interface = metrics.NewMetricsExporter(watcher)
	var labelsExporter export.Interface = labels.NewLabelsExporter(watcher, kubeclient)

	go watcher.Watch(stopper)
	go metricExporter.Run(stopper)
	go labelsExporter.Run(stopper)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	s := <-sig
	log.Printf("Received signal \"%v\"\n", s)
	close(stopper)
}
