package app

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/go-playground/validator"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
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
	appRunner.loadConfig()
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

func (appRunner *AppRunner) loadConfig() {
	config := appRunner.App.GetConfig()
	if config == nil {
		return
	}
	err := BindStruct(config)
	if err != nil {
		log.Fatal("Error binding environment variables")
	}

	viper.AutomaticEnv()
	err = viper.Unmarshal(&config)
	if err != nil {
		log.Fatalf("unable to unmarshall the config %v", err)
	}

	validate := validator.New()
	if err := validate.Struct(config); err != nil {
		log.Fatalf("Missing required attributes %v\n", err)
	}
}

// patch for viper to bind all relevant envs, from here: https://github.com/spf13/viper/pull/1429
// scan be deleted on feuture versions of viper
func BindStruct(input interface{}) error {
	envKeysMap := map[string]interface{}{}
	if err := mapstructure.Decode(input, &envKeysMap); err != nil {
		return err
	}

	for key, _ := range envKeysMap {
		if err := viper.BindEnv(key); err != nil {
			return err
		}
	}

	return nil
}