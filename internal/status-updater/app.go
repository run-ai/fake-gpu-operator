package status_updater

import (
	"sync"

	"github.com/run-ai/fake-gpu-operator/internal/common/config"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/handle"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/inform"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var InClusterConfigFn = rest.InClusterConfig
var KubeClientFn = func(c *rest.Config) kubernetes.Interface {
	return kubernetes.NewForConfigOrDie(c)
}

var DynamicClientFn = func(c *rest.Config) dynamic.Interface {
	return dynamic.NewForConfigOrDie(c)
}

type StatusUpdaterApp struct {
	Informer inform.Interface
	Handler  handle.Interface
}

func NewStatusUpdaterApp() *StatusUpdaterApp {
	return &StatusUpdaterApp{}
}

func (app *StatusUpdaterApp) Start(stopper chan struct{}, wg *sync.WaitGroup) {
	app.Init()

	wg.Add(2)
	go app.Handler.Run(stopper, wg)
	go app.Informer.Run(stopper, wg)
}

func (app *StatusUpdaterApp) Init() {
	requiredEnvVars := []string{"TOPOLOGY_CM_NAME", "TOPOLOGY_CM_NAMESPACE"}
	config.ValidateConfig(requiredEnvVars)

	config, err := InClusterConfigFn()
	if err != nil {
		panic(err.Error())
	}
	kubeclient := KubeClientFn(config)
	dynamicClient := DynamicClientFn(config)

	app.Informer = inform.NewInformer(kubeclient)
	app.Handler = handle.NewPodEventHandler(kubeclient, dynamicClient, app.Informer)
}

func (app *StatusUpdaterApp) Name() string {
	return "StatusUpdater"
}
