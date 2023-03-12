package app_test

import (
	"testing"
	"time"

	"github.com/run-ai/fake-gpu-operator/internal/common/app"
	"github.com/stretchr/testify/assert"
)

type FakeApp struct {
	name    bool
	stopped bool
	config  bool
	init    bool
	stopCh  chan struct{}
}

func (fa *FakeApp) Run() {
	<-fa.stopCh
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

func (fa *FakeApp) Init(stop chan struct{}) {
	fa.stopCh = stop
	fa.init = true
}

func TestRunnerStopsOnSignal(t *testing.T) {
	runner := app.NewAppRunner(&FakeApp{})
	wait := make(chan struct{})
	go func() {
		runner.Run()
		close(wait)
	}()

	time.Sleep(10 * time.Millisecond)
	runner.Stop()

	select {
	case <-wait:
		return
	case <-time.After(100 * time.Millisecond):
		t.Errorf("app runner failed to close")
	}
}

func TestAllAppFunctionsCall(t *testing.T) {
	fa := &FakeApp{}
	runner := app.NewAppRunner(fa)
	wait := make(chan struct{})
	go func() {
		runner.Run()
		close(wait)
	}()

	time.Sleep(10 * time.Millisecond)
	runner.Stop()

	<-wait
	assert.True(t, fa.name)
	assert.True(t, fa.stopped)
	assert.True(t, fa.config)
	assert.True(t, fa.init)
}
