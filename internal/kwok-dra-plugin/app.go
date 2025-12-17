package kwokdraplugin

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

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	cmcontroller "github.com/run-ai/fake-gpu-operator/internal/kwok-dra-plugin/controllers/configmap"
)

type KWOKDraPluginAppConfiguration struct {
	TopologyCmName      string `mapstructure:"TOPOLOGY_CM_NAME" validate:"required"`
	TopologyCmNamespace string `mapstructure:"TOPOLOGY_CM_NAMESPACE" validate:"required"`
}

type KWOKDraPluginApp struct {
	mgr    ctrl.Manager
	stopCh chan struct{}
}

func (app *KWOKDraPluginApp) Run() {
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

func (app *KWOKDraPluginApp) Init(stopCh chan struct{}) {
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
	if err := resourceapi.AddToScheme(scheme); err != nil {
		log.Fatalf("Failed to add resource.k8s.io to scheme: %v", err)
	}

	// Create manager
	namespace := viper.GetString(constants.EnvTopologyCmNamespace)
	topologyCMName := viper.GetString(constants.EnvTopologyCmName)
	app.mgr, err = ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		log.Fatalf("Failed to create manager: %v", err)
	}

	// Create kubernetes client for ResourceSlice operations (uses client-go interface)
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalf("Failed to create kubernetes client: %v", err)
	}

	// Setup ConfigMap reconciler
	if err := cmcontroller.SetupWithManager(app.mgr, kubeClient, namespace, topologyCMName); err != nil {
		log.Fatalf("Failed to setup ConfigMap controller: %v", err)
	}
}

func (app *KWOKDraPluginApp) Name() string {
	return "KWOKDraPlugin"
}

func (app *KWOKDraPluginApp) GetConfig() interface{} {
	var config KWOKDraPluginAppConfiguration
	return config
}
