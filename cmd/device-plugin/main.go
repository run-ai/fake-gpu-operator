package main

import (
	"log"
	"os"
	"os/signal"
	"path"
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
	initPreloaders()

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
	publish("/bin/nvidia-smi", "/runai/bin/nvidia-smi")
}

func initPreloaders() {
	publish("/shared/memory/preloader.so", "/runai/shared/memory/preloader.so")
	publish("/shared/pid/preloader.so", "/runai/shared/pid/preloader.so")
}

func publish(srcFile string, destFile string) {
	srcFileInfo, err := os.Stat(srcFile)
	if os.IsNotExist(err) {
		log.Printf("%s not found in %s\n", path.Base(srcFile), srcFile)
		return
	}

	if destFileInfo, err := os.Stat(destFile); os.IsNotExist(err) || destFileInfo.ModTime().Before(srcFileInfo.ModTime()) {
		log.Printf("%s is missing or outdated on the host, copying it from /runai/bin\n", destFile)
		err = copy.Copy(srcFile, destFile)
		if err != nil {
			log.Printf("Failed to copy %s: %s\n", srcFile, err)
		}
	}
}
