package status_updater

import (
	"sync"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers"
	nodecontroller "github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers/node"
	podcontroller "github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers/pod"
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

	app.wg = &sync.WaitGroup{}

	app.kubeClient = KubeClientFn(clusterConfig)
	dynamicClient := DynamicClientFn(clusterConfig)

	app.Controllers = append(app.Controllers, podcontroller.NewPodController(app.kubeClient, dynamicClient, app.wg))
	app.Controllers = append(app.Controllers, nodecontroller.NewNodeController(app.kubeClient, app.wg))
}

func (app *StatusUpdaterApp) Name() string {
	return "StatusUpdater"
}

func (app *StatusUpdaterApp) GetConfig() interface{} {
	var config StatusUpdaterAppConfiguration

	return config
}
