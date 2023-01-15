package main

import (
	"github.com/run-ai/fake-gpu-operator/internal/common/app"
	"github.com/run-ai/fake-gpu-operator/internal/migfaker"
)

func main() {
	appRunner := app.NewAppRunner(&migfaker.MigFakeApp{})
	appRunner.Run()
}
