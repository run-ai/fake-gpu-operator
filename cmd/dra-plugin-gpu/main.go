package main

import (
	"github.com/run-ai/fake-gpu-operator/internal/common/app"
	dra_plugin_gpu "github.com/run-ai/fake-gpu-operator/internal/dra-plugin-gpu"
)

func main() {
	appRunner := app.NewAppRunner(&dra_plugin_gpu.DraPluginGpuApp{})
	appRunner.Run()
}
