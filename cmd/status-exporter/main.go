package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/run-ai/gpu-mock-stack/internal/common/config"
	"github.com/run-ai/gpu-mock-stack/internal/status-exporter/export"
	"github.com/run-ai/gpu-mock-stack/internal/status-exporter/export/labels"
	"github.com/run-ai/gpu-mock-stack/internal/status-exporter/export/metrics"
	"github.com/run-ai/gpu-mock-stack/internal/status-exporter/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	log.Println("Fake Status Exporter Running")

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

	// Wait for
	go watcher.Watch(stopper)
	go metricExporter.Run(stopper)
	go labelsExporter.Run(stopper)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	s := <-sig
	log.Printf("Received signal \"%v\"\n", s)
	close(stopper)
}
