package computedomaindraplugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/run-ai/fake-gpu-operator/internal/common/app"
	"github.com/run-ai/fake-gpu-operator/pkg/compute-domain/consts"
	"go.uber.org/zap/zapcore"
	coreclientset "k8s.io/client-go/kubernetes"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/dra-example-driver/pkg/flags"
)

const (
	DriverPluginCheckpointFile = "computedomain-checkpoint.json"
)

// Internal types for backward compatibility with existing code
type Flags struct {
	kubeClientConfig flags.KubeClientConfig
	loggingConfig    *flags.LoggingConfig

	nodeName                      string
	cdiRoot                       string
	kubeletRegistrarDirectoryPath string
	kubeletPluginsDirectoryPath   string
	healthcheckPort               int
}

type Config struct {
	flags         *Flags
	coreclient    coreclientset.Interface
	cancelMainCtx func(error)
}

func (c Config) DriverPluginPath() string {
	return filepath.Join(c.flags.kubeletPluginsDirectoryPath, consts.ComputeDomainDriverName)
}

// AppConfig follows the same pattern as the controller app
type AppConfig struct {
	// Required flags
	NodeName string `mapstructure:"NODE_NAME"`

	// Optional flags with defaults
	CDIRoot                       string `mapstructure:"CDI_ROOT"`
	KubeletRegistrarDirectoryPath string `mapstructure:"KUBELET_REGISTRAR_DIRECTORY_PATH"`
	KubeletPluginsDirectoryPath   string `mapstructure:"KUBELET_PLUGINS_DIRECTORY_PATH"`
	HealthcheckPort               int    `mapstructure:"HEALTHCHECK_PORT"`
}

type ComputeDomainDRAPluginApp struct {
	config *AppConfig
	stop   chan struct{}
}

func NewComputeDomainDRAPluginApp() *ComputeDomainDRAPluginApp {
	return &ComputeDomainDRAPluginApp{}
}

func (app *ComputeDomainDRAPluginApp) Name() string {
	return "ComputeDomainDRAPlugin"
}

func (app *ComputeDomainDRAPluginApp) GetConfig() interface{} {
	if app.config == nil {
		app.config = &AppConfig{
			CDIRoot:                       "/etc/cdi",
			KubeletRegistrarDirectoryPath: kubeletplugin.KubeletRegistryDir,
			KubeletPluginsDirectoryPath:   kubeletplugin.KubeletPluginsDir,
			HealthcheckPort:               -1,
		}
	}
	return app.config
}

func (app *ComputeDomainDRAPluginApp) Init(stop chan struct{}) {
	app.stop = stop
}

func (app *ComputeDomainDRAPluginApp) Run() {
	// Set up logging
	opts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	klog.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle stop signal
	go func() {
		<-app.stop
		cancel()
	}()

	if err := app.runPlugin(ctx); err != nil {
		klog.Error(err, "plugin exited with error")
	}
}

func (app *ComputeDomainDRAPluginApp) runPlugin(ctx context.Context) error {
	logger := klog.FromContext(ctx)

	// Validate required config
	if app.config.NodeName == "" {
		return fmt.Errorf("node-name is required")
	}

	// Set up kube client config
	kubeClientConfig := flags.KubeClientConfig{}
	clientSets, err := kubeClientConfig.NewClientSets()
	if err != nil {
		return fmt.Errorf("create client: %v", err)
	}

	// Create internal config compatible with existing code
	config := &Config{
		flags: &Flags{
			kubeClientConfig:              kubeClientConfig,
			loggingConfig:                 flags.NewLoggingConfig(),
			nodeName:                      app.config.NodeName,
			cdiRoot:                       app.config.CDIRoot,
			kubeletRegistrarDirectoryPath: app.config.KubeletRegistrarDirectoryPath,
			kubeletPluginsDirectoryPath:   app.config.KubeletPluginsDirectoryPath,
			healthcheckPort:               app.config.HealthcheckPort,
		},
		coreclient: clientSets.Core,
	}

	err = os.MkdirAll(config.DriverPluginPath(), 0750)
	if err != nil {
		return err
	}

	info, err := os.Stat(config.flags.cdiRoot)
	switch {
	case err != nil && os.IsNotExist(err):
		err := os.MkdirAll(config.flags.cdiRoot, 0750)
		if err != nil {
			return err
		}
	case err != nil:
		return err
	case !info.IsDir():
		return fmt.Errorf("path for cdi file generation is not a directory: '%v'", err)
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()
	ctx, cancel := context.WithCancelCause(ctx)
	config.cancelMainCtx = cancel

	driver, err := NewComputeDomainDriver(ctx, config)
	if err != nil {
		return err
	}

	<-ctx.Done()
	// restore default signal behavior as soon as possible in case graceful
	// shutdown gets stuck.
	stop()
	if err := context.Cause(ctx); err != nil && !errors.Is(err, context.Canceled) {
		// A canceled context is the normal case here when the process receives
		// a signal. Only log the error for more interesting cases.
		logger.Error(err, "error from context")
	}

	err = driver.Shutdown(logger)
	if err != nil {
		logger.Error(err, "Unable to cleanly shutdown driver")
	}

	return nil
}

// Verify that ComputeDomainDRAPluginApp implements the App interface
var _ app.App = &ComputeDomainDRAPluginApp{}