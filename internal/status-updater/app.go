package status_updater

import (
	"sync"
	"time"

	"github.com/spf13/viper"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers"
	nodecontroller "github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers/node"
	podcontroller "github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers/pod"
	runaicontroller "github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers/runai"
)

var InClusterConfigFn = ctrl.GetConfigOrDie
var KubeClientFn = func(c *rest.Config) kubernetes.Interface {
	return kubernetes.NewForConfigOrDie(c)
}

var DynamicClientFn = func(c *rest.Config) dynamic.Interface {
	return dynamic.NewForConfigOrDie(c)
}

type StatusUpdaterAppConfiguration struct {
	TopologyCmName      string `mapstructure:"TOPOLOGY_CM_NAME" validate:"required"`
	TopologyCmNamespace string `mapstructure:"TOPOLOGY_CM_NAMESPACE" validate:"required"`
	PrometheusURL       string `mapstructure:"PROMETHEUS_URL"`
	DisableNodeLabeling            bool   `mapstructure:"DISABLE_NODE_LABELING"`
	RunaiIntegrationEnabled        bool   `mapstructure:"RUNAI_INTEGRATION_ENABLED"`
	RunaiIntegrationPollingInterval string `mapstructure:"RUNAI_INTEGRATION_POLLING_INTERVAL"`
}

type StatusUpdaterApp struct {
	Controllers []controllers.Interface
	kubeClient  kubernetes.Interface
	stopCh      chan struct{}
	wg          *sync.WaitGroup
}

func (app *StatusUpdaterApp) Run() {
	app.wg.Add(len(app.Controllers))
	for _, controller := range app.Controllers {
		go func(controller controllers.Interface) {
			defer app.wg.Done()
			controller.Run(app.stopCh)
		}(controller)
	}

	app.wg.Wait()
}

func (app *StatusUpdaterApp) Init(stopCh chan struct{}) {
	app.stopCh = stopCh

	// Initialize Prometheus configuration
	prometheusURL := viper.GetString(constants.EnvPrometheusURL)
	topology.InitPrometheusConfig(prometheusURL)

	clusterConfig := InClusterConfigFn()
	clusterConfig.QPS = 100
	clusterConfig.Burst = 200

	app.wg = &sync.WaitGroup{}

	app.kubeClient = KubeClientFn(clusterConfig)
	dynamicClient := DynamicClientFn(clusterConfig)

	disableNodeLabeling := viper.GetBool(constants.EnvDisableNodeLabeling)

	app.Controllers = append(app.Controllers, podcontroller.NewPodController(app.kubeClient, dynamicClient, app.wg))
	app.Controllers = append(app.Controllers, nodecontroller.NewNodeController(app.kubeClient, app.wg, disableNodeLabeling))

	if viper.GetBool(constants.EnvRunaiIntegrationEnabled) {
		intervalStr := viper.GetString(constants.EnvRunaiIntegrationPollingInterval)
		interval, err := time.ParseDuration(intervalStr)
		if err != nil || interval == 0 {
			interval = 30 * time.Second
		}
		app.Controllers = append(app.Controllers,
			runaicontroller.NewRunaiController(app.kubeClient, dynamicClient, interval))
	}
}

func (app *StatusUpdaterApp) Name() string {
	return "StatusUpdater"
}

func (app *StatusUpdaterApp) GetConfig() interface{} {
	var config StatusUpdaterAppConfiguration

	return config
}
