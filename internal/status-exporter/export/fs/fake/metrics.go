package fake

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// core_team_metric.NewCoreTeamMetric(
	// 	"runai_pod_gpu_utilization",
	// 	"GPU Utilization of Pod",
	// 	coreTeamMetricsDir,
	// 	"pod/{pod_uuid}/metrics/gpu/{gpu}/utilization.sm"),
	// core_team_metric.NewCoreTeamMetric(
	// 	"runai_pod_gpu_memory_used_bytes",
	// 	"GPU Memory Usage of Pod in Bytes",
	// 	coreTeamMetricsDir,
	// 	"pod/{pod_uuid}/metrics/gpu/{gpu}/memory.allocated"),
	// core_team_metric.NewCoreTeamMetric(
	// 	"runai_pod_gpu_swap_ram_used_bytes",
	// 	"GPU Swap Ram Memory Usage of Pod in Bytes",
	// 	coreTeamMetricsDir,
	// 	"pod/{pod_uuid}/metrics/gpu/{gpu}/memory.swap_ram_used"),
	// core_team_metric.NewCoreTeamMetric(
	// 	"runai_gpu_oomkill_burst_count",
	// 	"GPU Burst OOMKill count",
	// 	coreTeamMetricsDir,
	// 	"metrics/gpu/{gpu}/oom.burst"),
	// core_team_metric.NewCoreTeamMetric(
	// 	"runai_gpu_oomkill_idle_count",
	// 	"GPU Idle OOMKill count",
	// 	coreTeamMetricsDir,
	// 	"metrics/gpu/{gpu}/oom.idle"),
	// core_team_metric.NewCoreTeamMetric(
	// 	"runai_gpu_oomkill_priority_count",
	// 	"GPU Priority OOMKill count",
	// 	coreTeamMetricsDir,
	// 	"metrics/gpu/{gpu}/oom.priority"),
	// core_team_metric.NewCoreTeamMetric(
	// 	"runai_gpu_oomkill_swap_out_of_ram_count",
	// 	"GPU swap out of RAM OOMKill count",
	// 	coreTeamMetricsDir,
	// 	"metrics/gpu/{gpu}/oom.swap_out_of_ram"),

	runaiPodGpuUtil = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "runai_pod_gpu_utilization",
		Help: "GPU Utilization of Pod",
	}, []string{"pod_uuid", "gpu"})

	runaiPodGpuMemoryUsedBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "runai_pod_gpu_memory_used_bytes",
		Help: "GPU Memory Usage of Pod in Bytes",
		
)
