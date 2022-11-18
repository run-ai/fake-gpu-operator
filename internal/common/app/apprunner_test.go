package app_test

import (
	"sync"
	"testing"
	"time"

	"github.com/run-ai/fake-gpu-operator/internal/common/app"
	"github.com/stretchr/testify/assert"
)

type FakeApp struct {
	name    bool
	stopped bool
	config  bool
}

func (fa *FakeApp) Start(stopper chan struct{}, wg *sync.WaitGroup) {
	<-stopper
	fa.stopped = true
}

func (fa *FakeApp) Name() string {
	fa.name = true
	return "FakeApp"
}

func (fa *FakeApp) GetConfig() interface{} {
	fa.config = true
	return nil
}

func TestRunnerStopsOnSignal(t *testing.T) {
	fa := &FakeApp{}
	runner := app.NewAppRunner(fa)
	go runner.RunApp()

	time.Sleep(10 * time.Millisecond)
	runner.Stop()
	time.Sleep(10 * time.Millisecond)

	assert.True(t, fa.name)
	assert.True(t, fa.stopped)
	assert.True(t, fa.config)
}
