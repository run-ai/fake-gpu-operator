package main

import (
	"github.com/run-ai/fake-gpu-operator/internal/common/app"
	migfaker "github.com/run-ai/fake-gpu-operator/internal/mig-faker"
)

func main() {
	migFakerApp := migfaker.NewMigFakeApp()
	appRunner := app.NewAppRunner(migFakerApp)
	appRunner.RunApp()
}
