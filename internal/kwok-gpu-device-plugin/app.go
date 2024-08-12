package kwokgdp

import (
	"sync"

	"k8s.io/client-go/dynamic"
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

var DynamicClientFn = func(c *rest.Config) dynamic.Interface {
	return dynamic.NewForConfigOrDie(c)
}

type StatusUpdaterAppConfiguration struct {
	TopologyCmName      string `mapstructure:"TOPOLOGY_CM_NAME" validate:"required"`
	TopologyCmNamespace string `mapstructure:"TOPOLOGY_CM_NAMESPACE" validate:"required"`
}

type StatusUpdaterApp struct {
	Controllers []controllers.Interface
	kubeClient  kubernetes.Interface
	stopCh      chan struct{}
	wg          *sync.WaitGroup
}

func (app *StatusUpdaterApp) Run() {
	app.wg.Add(len(app.Controllers))
	for _, controller := range app.Controllers {
		go func(controller controllers.Interface) {
			defer app.wg.Done()
			controller.Run(app.stopCh)
		}(controller)
	}

	app.wg.Wait()
}

func (app *StatusUpdaterApp) Init(stopCh chan struct{}) {
	app.stopCh = stopCh

	clusterConfig := InClusterConfigFn()
	clusterConfig.QPS = 100
	clusterConfig.Burst = 200

	app.wg = &sync.WaitGroup{}

	app.kubeClient = KubeClientFn(clusterConfig)

	app.Controllers = append(
		app.Controllers, cmcontroller.NewConfigMapController(
			app.kubeClient, viper.GetString(constants.EnvTopologyCmNamespace),
		),
	)
}

func (app *StatusUpdaterApp) Name() string {
	return "StatusUpdater"
}

func (app *StatusUpdaterApp) GetConfig() interface{} {
	var config StatusUpdaterAppConfiguration

	return config
}
