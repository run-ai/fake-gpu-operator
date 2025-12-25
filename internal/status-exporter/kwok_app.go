package status_exporter

import (
	"context"
	"log"

	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/labels"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/metrics"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"
)

type KWOKStatusExporterAppConfig struct {
	TopologyCmName      string `mapstructure:"TOPOLOGY_CM_NAME" validator:"required"`
	TopologyCmNamespace string `mapstructure:"TOPOLOGY_CM_NAMESPACE" validator:"required"`
}

type KWOKStatusExporterApp struct {
	mgr             ctrl.Manager
	metricsExporter *metrics.MultiNodeMetricsExporter
	stopCh          chan struct{}
}

func (app *KWOKStatusExporterApp) Run() {
	// Start metrics exporter ticker in background
	go app.metricsExporter.Run(app.stopCh)

	// Start controller-runtime manager
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle stop channel
	go func() {
		<-app.stopCh
		cancel()
	}()

	if err := app.mgr.Start(ctx); err != nil {
		log.Fatalf("Failed to start manager: %v", err)
	}
}

func (app *KWOKStatusExporterApp) Init(stopCh chan struct{}) {
	app.stopCh = stopCh

	// Initialize controller-runtime logger using klog
	ctrl.SetLogger(klog.NewKlogr())

	// Get controller-runtime config
	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.Fatalf("Failed to get config: %v", err)
	}
	cfg.QPS = 100
	cfg.Burst = 200

	// Create scheme with required types
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		log.Fatalf("Failed to add corev1 to scheme: %v", err)
	}

	// Create manager
	app.mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		log.Fatalf("Failed to create manager: %v", err)
	}

	// Create kubernetes client for label operations
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Failed to create kubernetes client: %v", err)
	}

	// Initialize Prometheus configuration
	prometheusURL := viper.GetString(constants.EnvPrometheusURL)
	if prometheusURL == "" {
		prometheusURL = "http://prometheus-operated.runai:9090"
	}
	topology.InitPrometheusConfig(prometheusURL)

	// Create exporters
	app.metricsExporter = metrics.NewMultiNodeMetricsExporter()
	labelsExporter := labels.NewMultiNodeLabelsExporter(kubeClient)

	// Setup multi-node watcher with exporters
	namespace := viper.GetString(constants.EnvTopologyCmNamespace)
	topologyCMName := viper.GetString(constants.EnvTopologyCmName)

	_, err = watch.SetupMultiNodeWatcherWithManager(app.mgr, namespace, topologyCMName, app.metricsExporter, labelsExporter)
	if err != nil {
		log.Fatalf("Failed to setup multi-node watcher: %v", err)
	}

	log.Println("KWOK Status Exporter initialized successfully")
}

func (app *KWOKStatusExporterApp) Name() string {
	return "KWOKStatusExporter"
}

func (app *KWOKStatusExporterApp) GetConfig() interface{} {
	var config KWOKStatusExporterAppConfig
	return config
}
