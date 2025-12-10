package dra_plugin_gpu

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/spf13/viper"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/klog/v2"
)

type DraPluginGpuAppConfig struct {
	NodeName                      string `mapstructure:"NODE_NAME" validate:"required"`
	CDIRoot                       string `mapstructure:"CDI_ROOT" validate:"required"`
	KubeletRegistrarDirectoryPath string `mapstructure:"KUBELET_REGISTRAR_DIRECTORY_PATH"`
	KubeletPluginsDirectoryPath   string `mapstructure:"KUBELET_PLUGINS_DIRECTORY_PATH"`
	HealthcheckPort               int    `mapstructure:"HEALTHCHECK_PORT"`
}

type DraPluginGpuApp struct {
	Config     DraPluginGpuAppConfig
	driver     *Driver
	kubeClient *kubeclient.KubeClient
	stopCh     chan struct{}
	ctx        context.Context
	cancel     context.CancelFunc
}

func (app *DraPluginGpuApp) GetConfig() interface{} {
	var config DraPluginGpuAppConfig
	return config
}

func (app *DraPluginGpuApp) Name() string {
	return "DraPluginGpuApp"
}

func (app *DraPluginGpuApp) Init(stop chan struct{}) {
	app.stopCh = stop

	if err := viper.Unmarshal(&app.Config); err != nil {
		log.Fatalf("failed to unmarshal configuration: %v", err)
	}

	// Set defaults for kubelet paths if not provided
	if app.Config.KubeletRegistrarDirectoryPath == "" {
		app.Config.KubeletRegistrarDirectoryPath = kubeletplugin.KubeletRegistryDir
	}
	if app.Config.KubeletPluginsDirectoryPath == "" {
		app.Config.KubeletPluginsDirectoryPath = kubeletplugin.KubeletPluginsDir
	}
	if app.Config.CDIRoot == "" {
		app.Config.CDIRoot = "/etc/cdi"
	}
	if app.Config.HealthcheckPort == 0 {
		app.Config.HealthcheckPort = -1 // Default to disabled
	}

	// Create context that will be cancelled when stopCh closes
	app.ctx, app.cancel = context.WithCancel(context.Background())

	// Start goroutine to cancel context when stopCh closes
	go func() {
		<-app.stopCh
		app.cancel()
	}()

	// Initialize kubeclient
	app.kubeClient = kubeclient.NewKubeClient(nil, stop)

	// Validate and create directories
	if err := app.validateAndCreateDirectories(); err != nil {
		log.Fatalf("Failed to validate/create directories: %v", err)
	}

	// Create internal config
	internalFlags := &Flags{
		NodeName:                      app.Config.NodeName,
		CDIRoot:                       app.Config.CDIRoot,
		KubeletRegistrarDirectoryPath: app.Config.KubeletRegistrarDirectoryPath,
		KubeletPluginsDirectoryPath:   app.Config.KubeletPluginsDirectoryPath,
		HealthcheckPort:               app.Config.HealthcheckPort,
	}

	config := &Config{
		Flags:      internalFlags,
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
	if err := SetupNodeController(app.ctx, app.driver.GetState(), app.Config.NodeName); err != nil {
		log.Fatalf("Failed to setup node controller: %v", err)
	}
}

func (app *DraPluginGpuApp) Run() {
	// Block until stopCh is closed
	<-app.stopCh

	// Cleanup/shutdown
	logger := klog.Background()
	if err := app.driver.Shutdown(logger); err != nil {
		logger.Error(err, "Unable to cleanly shutdown driver")
	}
}

func (app *DraPluginGpuApp) validateAndCreateDirectories() error {
	// Create driver plugin directory
	config := &Config{
		Flags: &Flags{
			KubeletPluginsDirectoryPath: app.Config.KubeletPluginsDirectoryPath,
		},
	}
	driverPluginPath := config.DriverPluginPath()

	err := os.MkdirAll(driverPluginPath, 0750)
	if err != nil {
		return fmt.Errorf("failed to create driver plugin directory: %w", err)
	}

	// Validate/create CDI root directory
	info, err := os.Stat(app.Config.CDIRoot)
	switch {
	case err != nil && os.IsNotExist(err):
		err := os.MkdirAll(app.Config.CDIRoot, 0750)
		if err != nil {
			return fmt.Errorf("failed to create CDI root directory: %w", err)
		}
	case err != nil:
		return fmt.Errorf("failed to stat CDI root directory: %w", err)
	case !info.IsDir():
		return fmt.Errorf("path for CDI file generation is not a directory: %s", app.Config.CDIRoot)
	}

	return nil
}
