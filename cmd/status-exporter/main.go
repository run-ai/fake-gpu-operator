package main

import (
	"github.com/run-ai/fake-gpu-operator/internal/common/app"
	status_exporter "github.com/run-ai/fake-gpu-operator/internal/status-exporter"
)

func main() {
	statusExporterApp := status_exporter.NewStatusExporterApp()
	appRunner := app.NewAppRunner(statusExporterApp)
	appRunner.RunApp()
}
