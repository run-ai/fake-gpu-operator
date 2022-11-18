package migfaker

import "sync"

type MigFakeAppConfig struct {
}
type MigFakeApp struct {
}

func NewMigFakeApp() *MigFakeApp {
	return &MigFakeApp{}
}

func (app *MigFakeApp) Start(stopper chan struct{}, wg *sync.WaitGroup) {
}

func (app *MigFakeApp) Name() string {
	return "MigFakeApp"
}

func (app *MigFakeApp) GetConfig() interface{} {
	var config MigFakeAppConfig
	return config
}
