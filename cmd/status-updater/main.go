package main

import (
	"github.com/run-ai/fake-gpu-operator/internal/common/app"
	status_updater "github.com/run-ai/fake-gpu-operator/internal/status-updater"
)

func main() {
	appRunner := app.NewAppRunner(&status_updater.StatusUpdaterApp{})
	appRunner.Run()
}
