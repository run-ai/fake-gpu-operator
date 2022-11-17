package migfaker

import "sync"

type MigFakeApp struct {
	stopper chan struct{}
}

func NewApp() *MigFakeApp {
	app := &MigFakeApp{
		stopper: make(chan struct{}),
	}
	return app
}

func (migApp *MigFakeApp) Start(stopper chan struct{}, wg *sync.WaitGroup) {
}
