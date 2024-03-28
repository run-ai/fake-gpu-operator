package main

import (
	"github.com/run-ai/fake-gpu-operator/internal/common/app"
	"github.com/run-ai/fake-gpu-operator/internal/common/config"
	status_updater "github.com/run-ai/fake-gpu-operator/internal/status-updater"
)

func main() {
	requiredEnvVars := []string{"TOPOLOGY_CM_NAME", "TOPOLOGY_CM_NAMESPACE", "FAKE_NODE_DEPLOYMENTS_PATH"}
	config.ValidateConfig(requiredEnvVars)

	appRunner := app.NewAppRunner(&status_updater.StatusUpdaterApp{})
	appRunner.Run()
}
