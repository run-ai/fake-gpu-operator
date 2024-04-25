package app

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/go-playground/validator"
	"github.com/mitchellh/mapstructure"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
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
	stopCh := make(chan struct{}, 1)
	LoadConfig(app)
	app.Init(stopCh)
	return &AppRunner{
		App:        app,
		stopSignal: stop,
		stopper:    stopCh,
		wg:         sync.WaitGroup{},
	}
}

func (appRunner *AppRunner) Run() {
	appRunner.wg.Add(1)
	go func() {
		defer appRunner.wg.Done()
		appRunner.App.Run()
	}()

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

func LoadConfig(app App) {
	config := app.GetConfig()
	if config == nil {
		return
	}
	err := bindStruct(config)
	if err != nil {
		log.Fatal("Error binding environment variables")
	}

	setDefaults()
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
func bindStruct(input interface{}) error {
	envKeysMap := map[string]interface{}{}
	if err := mapstructure.Decode(input, &envKeysMap); err != nil {
		return err
	}

	for key := range envKeysMap {
		if err := viper.BindEnv(key); err != nil {
			return err
		}
	}

	return nil
}

func setDefaults() {
	viper.SetDefault(constants.EnvTopologyCmName, "topology")
	viper.SetDefault(constants.EnvTopologyCmNamespace, "gpu-operator")
}
