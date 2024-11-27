package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	gpuUtilization = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "DCGM_FI_DEV_GPU_UTIL",
		Help: "GPU Utilization",
	}, []string{"gpu", "UUID", "device", "modelName", "Hostname", "container", "namespace", "pod", "instance"})
	gpuFbUsed = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "DCGM_FI_DEV_FB_USED",
		Help: "GPU Framebuffer Used",
	}, []string{"gpu", "UUID", "device", "modelName", "Hostname", "container", "namespace", "pod", "instance"})
	gpuFbFree = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "DCGM_FI_DEV_FB_FREE",
		Help: "GPU Framebuffer Free",
	}, []string{"gpu", "UUID", "device", "modelName", "Hostname", "container", "namespace", "pod", "instance"})
)
