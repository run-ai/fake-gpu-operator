package main

import (
	"log"

	status_updater "github.com/run-ai/fake-gpu-operator/internal/status-updater"
)

func main() {
	log.Println("Fake Status Updater Running")

	app := status_updater.NewApp()
	app.Run()
}
