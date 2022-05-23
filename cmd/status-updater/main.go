package main

import (
	"log"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	status_updater "github.com/run-ai/fake-gpu-operator/internal/status-updater"
)

var InClusterConfigFn = rest.InClusterConfig
var KubeClientFn = func(c *rest.Config) kubernetes.Interface {
	return kubernetes.NewForConfigOrDie(c)
}

func main() {
	log.Println("Fake Status Updater Running")

	status_updater.Run()
}
