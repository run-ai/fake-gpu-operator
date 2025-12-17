package main

import (
	"github.com/run-ai/fake-gpu-operator/internal/common/app"
	"github.com/run-ai/fake-gpu-operator/internal/common/config"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	kwokdraplugin "github.com/run-ai/fake-gpu-operator/internal/kwok-dra-plugin"
)

func main() {
	requiredEnvVars := []string{constants.EnvTopologyCmName, constants.EnvTopologyCmNamespace, constants.EnvFakeGpuOperatorNs}
	config.ValidateConfig(requiredEnvVars)

	appRunner := app.NewAppRunner(&kwokdraplugin.KWOKDraPluginApp{})
	appRunner.Run()
}
