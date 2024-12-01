package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/spf13/viper"
)

var (
	gpuUtilization *prometheus.GaugeVec
	gpuFbUsed      *prometheus.GaugeVec
	gpuFbFree      *prometheus.GaugeVec

	once sync.Once
)

func initMetrics() {
	once.Do(func() {
		labelNames := getLabelNames()

		gpuUtilization = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "DCGM_FI_DEV_GPU_UTIL",
			Help: "GPU Utilization",
		}, labelNames)
		gpuFbUsed = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "DCGM_FI_DEV_FB_USED",
			Help: "GPU Framebuffer Used",
		}, labelNames)
		gpuFbFree = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "DCGM_FI_DEV_FB_FREE",
			Help: "GPU Framebuffer Free",
		}, labelNames)
	})
}

func getLabelNames() []string {
	labelNames := []string{"gpu", "UUID", "device", "modelName", "Hostname", "container", "namespace", "pod"}
	if viper.GetBool(constants.EnvExportPrometheusLabelEnrichments) {
		labelNames = append(labelNames, "instance", "exported_container", "exported_namespace", "exported_pod", "job", "service")
	}

	return labelNames
}
