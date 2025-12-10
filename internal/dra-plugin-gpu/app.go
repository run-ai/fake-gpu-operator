package dra_plugin_gpu

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/spf13/viper"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
)

type DraPluginGpuApp struct {
	Flags      Flags
	driver     *Driver
	kubeClient *kubeclient.KubeClient
	stopCh     chan struct{}
	ctx        context.Context
	cancel     context.CancelFunc
}

func (app *DraPluginGpuApp) GetConfig() interface{} {
	return Flags{}
}

func (app *DraPluginGpuApp) Name() string {
	return "DraPluginGpuApp"
}

func (app *DraPluginGpuApp) Init(stop chan struct{}) {
	app.stopCh = stop

	if err := viper.Unmarshal(&app.Flags); err != nil {
		log.Fatalf("failed to unmarshal configuration: %v", err)
	}

	// Set defaults
	if app.Flags.KubeletRegistrarDirectoryPath == "" {
		app.Flags.KubeletRegistrarDirectoryPath = kubeletplugin.KubeletRegistryDir
	}
	if app.Flags.KubeletPluginsDirectoryPath == "" {
		app.Flags.KubeletPluginsDirectoryPath = kubeletplugin.KubeletPluginsDir
	}
	if app.Flags.CDIRoot == "" {
		app.Flags.CDIRoot = "/etc/cdi"
	}
	if app.Flags.HealthcheckPort == 0 {
		app.Flags.HealthcheckPort = -1
	}

	app.ctx, app.cancel = context.WithCancel(context.Background())
	go func() {
		<-app.stopCh
		app.cancel()
	}()

	app.kubeClient = kubeclient.NewKubeClient(nil, stop)

	if err := app.validateAndCreateDirectories(); err != nil {
		log.Fatalf("Failed to validate/create directories: %v", err)
	}

	config := &Config{
		Flags:      &app.Flags,
		CoreClient: app.kubeClient.ClientSet,
		CancelMainCtx: func(err error) {
			log.Printf("Fatal error occurred: %v", err)
			app.cancel()
		},
	}

	// Initialize driver
	var err error
	app.driver, err = NewDriver(app.ctx, config)
	if err != nil {
		log.Fatalf("Failed to create driver: %v", err)
	}

	// Set up node controller to watch for annotation changes
	if err := SetupNodeController(app.ctx, app.driver.GetState(), app.Flags.NodeName); err != nil {
		log.Fatalf("Failed to setup node controller: %v", err)
	}
}

func (app *DraPluginGpuApp) Run() {
	<-app.stopCh

	if err := app.driver.Shutdown(); err != nil {
		log.Printf("Unable to cleanly shutdown driver: %v", err)
	}
}

func (app *DraPluginGpuApp) validateAndCreateDirectories() error {
	config := &Config{Flags: &app.Flags}
	if err := os.MkdirAll(config.DriverPluginPath(), 0750); err != nil {
		return fmt.Errorf("failed to create driver plugin directory: %w", err)
	}
	if err := os.MkdirAll(app.Flags.CDIRoot, 0750); err != nil {
		return fmt.Errorf("failed to create CDI root directory: %w", err)
	}
	return nil
}
