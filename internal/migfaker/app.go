package migfaker

import (
	"log"
	"sync"

	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/kubernetes"
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
	Clientset         *kubernetes.Clientset
	MigFaker          *MigFaker
}

func (app *MigFakeApp) Start(stop chan struct{}, wg *sync.WaitGroup) {
	ContinuouslySyncMigConfigChanges(app.Clientset, app.SyncableMigConfig, stop)
	app.MigFaker.FakeNodeLabels()
	for {
		log.Printf("Waiting for change to '%s' annotation", MigConfigAnnotation)
		value := app.SyncableMigConfig.Get()
		log.Printf("Updating to MIG config: %s", value)
		var migConfig AnnotationMigConfig
		yaml.Unmarshal([]byte(value), &migConfig)
		app.MigFaker.FakeMapping(&migConfig.MigConfigs)
		log.Printf("Successfuly updated MIG config")
	}
}

func (app *MigFakeApp) Init() {
	viper.Unmarshal(&app.Config)

	config, err := clientcmd.BuildConfigFromFlags("", app.Config.KubeConfig)
	if err != nil {
		log.Fatalf("error building kubernetes clientcmd config: %s", err)
	}

	app.Clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("error building kubernetes clientset from config: %s", err)
	}

	app.SyncableMigConfig = NewSyncableMigConfig()

	app.MigFaker = NewMigFaker(kubeclient.NewKubeClient(app.Clientset))
}

func (app *MigFakeApp) Name() string {
	return "MigFakeApp"
}

func (app *MigFakeApp) GetConfig() interface{} {
	var config MigFakeAppConfig
	return config
}
