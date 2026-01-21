package main

import (
	"github.com/run-ai/fake-gpu-operator/internal/common/app"
	"github.com/run-ai/fake-gpu-operator/internal/common/config"
	kwokcomputedomaindraplugin "github.com/run-ai/fake-gpu-operator/internal/kwok-compute-domain-dra-plugin"
)

func main() {
	requiredEnvVars := []string{kwokcomputedomaindraplugin.EnvFakeGpuOperatorNamespace}
	config.ValidateConfig(requiredEnvVars)

	appRunner := app.NewAppRunner(&kwokcomputedomaindraplugin.KWOKComputeDomainDraPluginApp{})
	appRunner.Run()
}
