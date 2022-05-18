package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/run-ai/gpu-mock-stack/internal/common/config"
	"github.com/run-ai/gpu-mock-stack/internal/common/topology"
	"github.com/run-ai/gpu-mock-stack/internal/deviceplugin"
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

	devicePlugin := deviceplugin.NewDevicePlugin(topology)
	devicePlugin.Serve()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	s := <-sig
	log.Printf("Received signal \"%v\"\n", s)
}
