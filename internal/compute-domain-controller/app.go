package computedomaincontroller

import (
	"context"
	"fmt"

	"go.uber.org/zap/zapcore"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	computedomainv1beta1 "github.com/NVIDIA/k8s-dra-driver-gpu/api/nvidia.com/resource/v1beta1"
	"github.com/run-ai/fake-gpu-operator/internal/common/app"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = resourceapi.AddToScheme(scheme)
	_ = computedomainv1beta1.AddToScheme(scheme)
}

type Config struct {
	MetricsBindAddress string `mapstructure:"METRICS_BIND_ADDRESS"`
	HealthProbeAddress string `mapstructure:"HEALTH_PROBE_BIND_ADDRESS"`
	LeaderElection     bool   `mapstructure:"LEADER_ELECT"`
}

type ComputeDomainApp struct {
	config *Config
	stop   chan struct{}
}

func NewComputeDomainApp() *ComputeDomainApp {
	return &ComputeDomainApp{}
}

func (app *ComputeDomainApp) Name() string {
	return "ComputeDomainController"
}

func (app *ComputeDomainApp) GetConfig() interface{} {
	if app.config == nil {
		app.config = &Config{
			MetricsBindAddress: ":8080",
			HealthProbeAddress: ":8081",
			LeaderElection:     false,
		}
	}
	return app.config
}

func (app *ComputeDomainApp) Init(stop chan struct{}) {
	app.stop = stop
}

func (app *ComputeDomainApp) Run() {
	opts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle stop signal
	go func() {
		<-app.stop
		cancel()
	}()

	if err := app.runController(ctx); err != nil {
		ctrl.Log.Error(err, "controller exited with error")
	}
}

func (app *ComputeDomainApp) runController(ctx context.Context) error {
	cfg := ctrl.GetConfigOrDie()

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: app.config.MetricsBindAddress,
		},
		HealthProbeBindAddress: app.config.HealthProbeAddress,
		LeaderElection:         app.config.LeaderElection,
		LeaderElectionID:       "fake-compute-domain-controller",
	})
	if err != nil {
		return fmt.Errorf("failed to create controller manager: %w", err)
	}

	reconciler := &ComputeDomainReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("failed to setup reconciler: %w", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("failed to add health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("failed to add ready check: %w", err)
	}

	ctrl.Log.Info("starting manager")
	return mgr.Start(ctx)
}

// Verify that ComputeDomainApp implements the App interface
var _ app.App = &ComputeDomainApp{}
