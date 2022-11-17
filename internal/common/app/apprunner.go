package app

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

type AppRunner struct {
	App        App
	stopSignal chan os.Signal
	stopper    chan struct{}
	wg         sync.WaitGroup
}

func NewAppRunner(app App) *AppRunner {
	stop := make(chan os.Signal, 1)
	return &AppRunner{
		App:        app,
		stopSignal: stop,
		stopper:    make(chan struct{}, 1),
		wg:         sync.WaitGroup{},
	}
}

func (appRunner *AppRunner) RunApp() {
	appRunner.App.Start(appRunner.stopper, &appRunner.wg)
	log.Printf("%s was Started", appRunner.App.Name())

	signal.Notify(appRunner.stopSignal, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	s := <-appRunner.stopSignal
	log.Printf("Received signal \"%v\"\n shuting down", s)

	close(appRunner.stopper)
	appRunner.wg.Wait()
	log.Printf("%s was Stopped", appRunner.App.Name())
}

func (appRunner *AppRunner) Stop() {
	appRunner.stopSignal <- os.Kill
}
