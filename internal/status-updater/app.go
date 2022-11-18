package status_updater

import (
	"sync"

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

type StatusUpdaterAppConfiguration struct {
	TopologyCmName      string `mapstructure:"TOPOLOGY_CM_NAME" validate:"required"`
	topologyCmNamespace string `mapstructure:"TOPOLOGY_CM_NAMESPACE" validate:"required"`
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

func (app *StatusUpdaterApp) GetConfig() interface{} {
	var config StatusUpdaterAppConfiguration
	return config
}
