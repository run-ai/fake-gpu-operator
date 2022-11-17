package status_updater

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

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
}

func NewStatusUpdaterApp() *StatusUpdaterApp {
	return &StatusUpdaterApp{}
}

func (app *StatusUpdaterApp) Start(stopper chan struct{}, wg *sync.WaitGroup) {
	requiredEnvVars := []string{"TOPOLOGY_CM_NAME", "TOPOLOGY_CM_NAMESPACE"}
	config.ValidateConfig(requiredEnvVars)

	config, err := InClusterConfigFn()
	if err != nil {
		panic(err.Error())
	}
	kubeclient := KubeClientFn(config)
	dynamicClient := DynamicClientFn(config)

	var informer inform.Interface = inform.NewInformer(kubeclient)
	var handler handle.Interface = handle.NewPodEventHandler(kubeclient, dynamicClient, informer)

	wg.Add(2)
	go handler.Run(stopper, wg)
	go informer.Run(stopper, wg)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	s := <-sig
	log.Printf("Received signal \"%v\"\n", s)
}

func (app *StatusUpdaterApp) Name() string {
	return "StatusUpdater"
}
