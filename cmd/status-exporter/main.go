package main

import (
	"log"

	status_exporter "github.com/run-ai/fake-gpu-operator/internal/status-exporter"
)

func main() {
	log.Println("Fake Status Exporter Running")

	app := status_exporter.NewApp()
	app.Run(nil)
}
