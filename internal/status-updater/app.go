package status_updater

import (
	"log"
	"os"
	"os/signal"
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

type App struct {
	stopper chan struct{}
}

func NewApp() *App {
	app := &App{
		stopper: make(chan struct{}),
	}
	return app
}

func (app *App) Run() {
	defer app.Stop()

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

	go handler.Run(app.stopper)
	go informer.Run(app.stopper)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	s := <-sig
	log.Printf("Received signal \"%v\"\n", s)
}

func (app *App) Stop() {
	close(app.stopper)
}
