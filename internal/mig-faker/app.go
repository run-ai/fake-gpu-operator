package migfaker

import "sync"

type MigFakeApp struct {
}

func NewMigFakeApp() *MigFakeApp {
	return &MigFakeApp{}
}

func (migApp *MigFakeApp) Start(stopper chan struct{}, wg *sync.WaitGroup) {
}

func (migApp *MigFakeApp) Name() string {
	return "MigFakeApp"
}
