package fake

import (
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

// FakeFsExporter exports fake filesystem based prometheus metrics.
type FakeFsExporter struct {
	topologyChan <-chan *topology.NodeTopology
}

func NewFakeFsExporter(topologyChan <-chan *topology.NodeTopology) *FakeFsExporter {
	return &FakeFsExporter{
		topologyChan: topologyChan,
	}
}

func (e *FakeFsExporter) Run(stopCh <-chan struct{}) {
	for {
		select {
		case nodeTopology := <-e.topologyChan:
			e.export(nodeTopology)
		case <-stopCh:
			return
		}
	}
}

func (e *FakeFsExporter) export(nodeTopology *topology.NodeTopology) {
	exportFsBasedMetrics(nodeTopology)
}

func exportFsBasedMetrics(nodeTopology *topology.NodeTopology) {
	// Export the following:
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

}
