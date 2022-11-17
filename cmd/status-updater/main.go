package main

import (
	"github.com/run-ai/fake-gpu-operator/internal/common/app"
	status_updater "github.com/run-ai/fake-gpu-operator/internal/status-updater"
)

func main() {
	statusUpdaterApp := status_updater.NewStatusUpdaterApp()
	appRunner := app.NewAppRunner(statusUpdaterApp)
	appRunner.RunApp()
}
