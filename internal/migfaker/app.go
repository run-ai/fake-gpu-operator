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
}

func (app *MigFakeApp) Start(wg *sync.WaitGroup) {
	ContinuouslySyncMigConfigChanges(app.KubeClient.ClientSet, app.SyncableMigConfig, app.stopCh)
	app.MigFaker.FakeNodeLabels()
	for {
		select {
		case <-app.stopCh:
			return
		default:
			log.Printf("Waiting for change to '%s' annotation", MigConfigAnnotation)
			value := app.SyncableMigConfig.Get()
			log.Printf("Updating to MIG config: %s", value)
			var migConfig AnnotationMigConfig
			yaml.Unmarshal([]byte(value), &migConfig)
			app.MigFaker.FakeMapping(&migConfig.MigConfigs)
			log.Printf("Successfuly updated MIG config")

		}
	}
}

func (app *MigFakeApp) Init(stop chan struct{}) {
	app.stopCh = stop

	viper.Unmarshal(&app.Config)
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
