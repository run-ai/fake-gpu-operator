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
	TopologyCmNamespace string `mapstructure:"TOPOLOGY_CM_NAMESPACE" validate:"required"`
}

type StatusUpdaterApp struct {
	Informer inform.Interface
	Handler  handle.Interface
	stopCh   chan struct{}
	wg       *sync.WaitGroup
}

func (app *StatusUpdaterApp) Start() {
	app.wg.Add(2)
	go app.Handler.Run(app.stopCh)
	go app.Informer.Run(app.stopCh)
}

func (app *StatusUpdaterApp) Init(stop chan struct{}, wg *sync.WaitGroup) {
	clusterConfig, err := InClusterConfigFn()
	if err != nil {
		panic(err.Error())
	}

	app.wg = wg

	kubeclient := KubeClientFn(clusterConfig)
	dynamicClient := DynamicClientFn(clusterConfig)

	app.Informer = inform.NewInformer(kubeclient, app.wg)
	app.Handler = handle.NewPodEventHandler(kubeclient, dynamicClient, app.Informer, app.wg)
}

func (app *StatusUpdaterApp) Name() string {
	return "StatusUpdater"
}

func (app *StatusUpdaterApp) GetConfig() interface{} {
	var config StatusUpdaterAppConfiguration
	return config
}
