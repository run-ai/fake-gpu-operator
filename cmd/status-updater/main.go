package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/run-ai/gpu-mock-stack/internal/common/config"
	"github.com/run-ai/gpu-mock-stack/internal/status-updater/handle"
	"github.com/run-ai/gpu-mock-stack/internal/status-updater/inform"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	log.Println("Fake Status Updater Running")

	requiredEnvVars := []string{"TOPOLOGY_CM_NAME", "TOPOLOGY_CM_NAMESPACE"}
	config.ValidateConfig(requiredEnvVars)

	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	kubeclient := kubernetes.NewForConfigOrDie(config)

	informer := inform.NewInformer(kubeclient)
	handler := handle.NewPodEventHandler(kubeclient, informer)

	stopper := make(chan struct{})
	go handler.Run(stopper)
	go informer.Run(stopper)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	s := <-sig
	log.Printf("Received signal \"%v\"\n", s)
	close(stopper)
}
