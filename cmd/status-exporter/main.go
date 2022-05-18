package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/run-ai/gpu-mock-stack/internal/status-exporter/export/labels"
	"github.com/run-ai/gpu-mock-stack/internal/status-exporter/export/metrics"
	"github.com/run-ai/gpu-mock-stack/internal/status-exporter/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	log.Println("Fake Status Exporter Running")

	validateEnvs()

	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	kubeclient := kubernetes.NewForConfigOrDie(config)

	// Watch for changes, and export metrics
	stopper := make(chan struct{})
	watcher := watch.NewKubeWatcher(kubeclient)
	metricExporter := metrics.NewMetricsExporter(watcher)
	labelsExporter := labels.NewLabelsExporter(watcher, kubeclient)

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

func validateEnvs() {
	requiredEnvs := []string{"NODE_NAME", "TOPOLOGY_CM_NAME", "TOPOLOGY_CM_NAMESPACE"}
	for _, env := range requiredEnvs {
		if os.Getenv(env) == "" {
			log.Printf("%s must be set\n", env)
			os.Exit(1)
		}
	}
}
