package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/otiai10/copy"
	"github.com/run-ai/fake-gpu-operator/internal/common/config"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/deviceplugin"
)

func main() {
	log.Println("Fake Device Plugin Running")

	requiredEnvVars := []string{"TOPOLOGY_PATH", "NODE_NAME"}
	config.ValidateConfig(requiredEnvVars)

	topology, err := topology.GetNodeTopologyFromFs(os.Getenv("TOPOLOGY_PATH"), os.Getenv("NODE_NAME"))
	if err != nil {
		log.Printf("Failed to get topology: %s\n", err)
		os.Exit(1)
	}

	initNvidiaSmi()

	devicePlugin := deviceplugin.NewDevicePlugin(topology)
	if err = devicePlugin.Serve(); err != nil {
		log.Printf("Failed to serve device plugin: %s\n", err)
		os.Exit(1)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	s := <-sig
	log.Printf("Received signal \"%v\"\n", s)
}

func initNvidiaSmi() {
	srcFileInfo, err := os.Stat("/bin/nvidia-smi")
	if os.IsNotExist(err) {
		log.Println("nvidia-smi not found in /bin/nvidia-smi")
		return
	}

	if destFileInfo, err := os.Stat("/runai/bin/nvidia-smi"); os.IsNotExist(err) || destFileInfo.ModTime().Before(srcFileInfo.ModTime()) {
		log.Println("nvidia-smi is missing or outdated on the host, copying it from /runai/bin")
		err = copy.Copy("/bin/nvidia-smi", "/runai/bin/nvidia-smi")
		if err != nil {
			log.Printf("Failed to copy nvidia-smi: %s\n", err)
		}
	}
}
