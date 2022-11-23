package migfaker

import (
	"log"
	"sync"

	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	MigConfigAnnotation = "nvidia.com/mig.config"
)

type MigFakeAppConfig struct {
	NodeName   string `mapstructure:"NODE_NAME" validator:"required"`
	KubeConfig string `mapstructure:"KUBECONFIG" validator:"required"`
}

type MigFakeApp struct {
	Config            MigFakeAppConfig
	SyncableMigConfig *SyncableMigConfig
	KubeClient        *kubeclient.KubeClient
	MigFaker          *MigFaker
	stopCh            chan struct{}
	wg                *sync.WaitGroup
}

func (app *MigFakeApp) Start() {
	ContinuouslySyncMigConfigChanges(app.KubeClient.ClientSet, app.SyncableMigConfig, app.stopCh)
	err := app.MigFaker.FakeNodeLabels()
	if err != nil {
		log.Fatalf("Error faking node labels: %e", err)
	}
	for {
		select {
		case <-app.stopCh:
			return
		default:
			log.Printf("Waiting for change to '%s' annotation", MigConfigAnnotation)
			value := app.SyncableMigConfig.Get()
			log.Printf("Updating to MIG config: %s", value)
			var migConfig AnnotationMigConfig
			err := yaml.Unmarshal([]byte(value), &migConfig)
			if err != nil {
				log.Printf("failed to unmarshal mig config: %e", err)
				break
			}
			err = app.MigFaker.FakeMapping(&migConfig.MigConfigs)
			if err != nil {
				log.Printf("Failed faking mig: %e", err)
			}
			log.Printf("Successfuly updated MIG config")

		}
	}
}

func (app *MigFakeApp) Init(stop chan struct{}, wg *sync.WaitGroup) {
	app.stopCh = stop
	app.wg = wg
	err := viper.Unmarshal(&app.Config)
	if err != nil {
		log.Fatalf("failed to unmarshal configuration: %e", err)
	}
	config, err := clientcmd.BuildConfigFromFlags("", app.Config.KubeConfig)
	if err != nil {
		log.Fatalf("error building kubernetes clientcmd config: %s", err)
	}

	app.KubeClient = kubeclient.NewKubeClient(config, stop)

	app.SyncableMigConfig = NewSyncableMigConfig()

	app.MigFaker = NewMigFaker(app.KubeClient)
}

func (app *MigFakeApp) Name() string {
	return "MigFakeApp"
}

func (app *MigFakeApp) GetConfig() interface{} {
	var config MigFakeAppConfig
	return config
}
