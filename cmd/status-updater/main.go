package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/run-ai/gpu-mock-stack/internal/status-updater/handle"
	"github.com/run-ai/gpu-mock-stack/internal/status-updater/inform"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	log.Println("Fake Status Updater Running")

	validateEnvs()

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

func validateEnvs() {
	requiredEnvs := []string{"TOPOLOGY_CM_NAME", "TOPOLOGY_CM_NAMESPACE"}
	for _, env := range requiredEnvs {
		if os.Getenv(env) == "" {
			log.Printf("%s must be set\n", env)
			os.Exit(1)
		}
	}
}
