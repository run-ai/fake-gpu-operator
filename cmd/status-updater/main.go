package main

import (
	"log"

	"github.com/run-ai/fake-gpu-operator/internal/common/app"
	status_updater "github.com/run-ai/fake-gpu-operator/internal/status-updater"
)

func main() {
	log.Println("Fake Status Updater Running")

	statusUpdaterApp := status_updater.NewStatusUpdaterApp()
	appRunner := app.NewAppRunner(statusUpdaterApp)
}
