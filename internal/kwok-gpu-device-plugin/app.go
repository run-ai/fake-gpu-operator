package kwokgdp

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/spf13/viper"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	cmcontroller "github.com/run-ai/fake-gpu-operator/internal/kwok-gpu-device-plugin/controllers/configmap"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers"
)

var InClusterConfigFn = ctrl.GetConfigOrDie
var KubeClientFn = func(c *rest.Config) kubernetes.Interface {
	return kubernetes.NewForConfigOrDie(c)
}

type StatusUpdaterAppConfiguration struct {
	TopologyCmName      string `mapstructure:"TOPOLOGY_CM_NAME" validate:"required"`
	TopologyCmNamespace string `mapstructure:"TOPOLOGY_CM_NAMESPACE" validate:"required"`
}

type KWOKDevicePluginApp struct {
	Controllers []controllers.Interface
	kubeClient  kubernetes.Interface
	stopCh      chan struct{}
}

func (app *KWOKDevicePluginApp) Run() {
	for _, controller := range app.Controllers {
		go func(controller controllers.Interface) {
			controller.Run(app.stopCh)
		}(controller)
	}
}

func (app *KWOKDevicePluginApp) Init(stopCh chan struct{}) {
	app.stopCh = stopCh

	clusterConfig := InClusterConfigFn()
	clusterConfig.QPS = 100
	clusterConfig.Burst = 200

	app.kubeClient = KubeClientFn(clusterConfig)

	app.Controllers = append(
		app.Controllers, cmcontroller.NewConfigMapController(
			app.kubeClient, viper.GetString(constants.EnvTopologyCmNamespace),
		),
	)
}

func (app *KWOKDevicePluginApp) Name() string {
	return "StatusUpdater"
}

func (app *KWOKDevicePluginApp) GetConfig() interface{} {
	var config StatusUpdaterAppConfiguration

	return config
}
