package main

import (
	"github.com/run-ai/fake-gpu-operator/internal/common/app"
	"github.com/run-ai/fake-gpu-operator/internal/common/config"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	status_updater "github.com/run-ai/fake-gpu-operator/internal/kwok-gpu-device-plugin"
)

func main() {
	requiredEnvVars := []string{constants.EnvTopologyCmName, constants.EnvTopologyCmNamespace, constants.EnvFakeGpuOperatorNs}
	config.ValidateConfig(requiredEnvVars)

	appRunner := app.NewAppRunner(&status_updater.KWOKDevicePluginApp{})
	appRunner.Run()
}
