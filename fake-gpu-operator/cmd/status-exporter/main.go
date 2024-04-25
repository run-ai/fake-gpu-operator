package main

import (
	"github.com/run-ai/fake-gpu-operator/internal/common/app"
	status_exporter "github.com/run-ai/fake-gpu-operator/internal/status-exporter"
)

func main() {
	appRunner := app.NewAppRunner(&status_exporter.StatusExporterApp{})
	appRunner.Run()
}
