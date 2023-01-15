package status_updater

import (
	"sync"

	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers"
	nodecontroller "github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers/node"
	podcontroller "github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers/pod"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	ctrl "sigs.k8s.io/controller-runtime"
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
	stopCh      chan struct{}
	wg          *sync.WaitGroup
}

func (app *StatusUpdaterApp) Run() {
	app.wg.Add(len(app.Controllers))
	for _, controller := range app.Controllers {
		go controller.Run(app.stopCh)
	}
}

func (app *StatusUpdaterApp) Init(stop chan struct{}, wg *sync.WaitGroup) {
	clusterConfig := InClusterConfigFn()

	app.wg = wg

	kubeClient := KubeClientFn(clusterConfig)
	dynamicClient := DynamicClientFn(clusterConfig)

	app.Controllers = append(app.Controllers, podcontroller.NewPodController(kubeClient, dynamicClient, app.wg))
	app.Controllers = append(app.Controllers, nodecontroller.NewNodeController(kubeClient, app.wg))
}

func (app *StatusUpdaterApp) Name() string {
	return "StatusUpdater"
}

func (app *StatusUpdaterApp) GetConfig() interface{} {
	var config StatusUpdaterAppConfiguration

	return config
}
