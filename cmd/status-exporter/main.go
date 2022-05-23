package main

import (
	"log"

	status_exporter "github.com/run-ai/gpu-mock-stack/internal/status-exporter"
)

func main() {
	log.Println("Fake Status Exporter Running")

	status_exporter.Run()
}
