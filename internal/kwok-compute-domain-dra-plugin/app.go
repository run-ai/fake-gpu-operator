package kwokcomputedomaindraplugin

import (
	"context"
	"log"

	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	nodecontroller "github.com/run-ai/fake-gpu-operator/internal/kwok-compute-domain-dra-plugin/controllers/node"
)

const (
	EnvFakeGpuOperatorNamespace = "FAKE_GPU_OPERATOR_NAMESPACE"
)

type KWOKComputeDomainDraPluginAppConfiguration struct {
	FakeGpuOperatorNamespace string `mapstructure:"FAKE_GPU_OPERATOR_NAMESPACE" validate:"required"`
}

type KWOKComputeDomainDraPluginApp struct {
	mgr    ctrl.Manager
	stopCh chan struct{}
}

func (app *KWOKComputeDomainDraPluginApp) Run() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-app.stopCh
		cancel()
	}()

	if err := app.mgr.Start(ctx); err != nil {
		log.Fatalf("Failed to start manager: %v", err)
	}
}

func (app *KWOKComputeDomainDraPluginApp) Init(stopCh chan struct{}) {
	app.stopCh = stopCh

	ctrl.SetLogger(klog.NewKlogr())

	cfg, err := ctrl.GetConfig()
	if err != nil {
		log.Fatalf("Failed to get config: %v", err)
	}
	cfg.QPS = 100
	cfg.Burst = 200

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		log.Fatalf("Failed to add corev1 to scheme: %v", err)
	}
	if err := resourceapi.AddToScheme(scheme); err != nil {
		log.Fatalf("Failed to add resource.k8s.io to scheme: %v", err)
	}

	namespace := viper.GetString(EnvFakeGpuOperatorNamespace)
	app.mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		log.Fatalf("Failed to create manager: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Failed to create kubernetes client: %v", err)
	}

	if err := nodecontroller.SetupWithManager(app.mgr, kubeClient, namespace); err != nil {
		log.Fatalf("Failed to setup Node controller: %v", err)
	}
}

func (app *KWOKComputeDomainDraPluginApp) Name() string {
	return "KWOKComputeDomainDraPlugin"
}

func (app *KWOKComputeDomainDraPluginApp) GetConfig() interface{} {
	var config KWOKComputeDomainDraPluginAppConfiguration
	return config
}
