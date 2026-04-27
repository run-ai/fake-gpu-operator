package component

import (
	"sort"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ReconcileParams holds configuration needed for resource generation.
type ReconcileParams struct {
	Namespace       string
	DefaultRegistry string
	FallbackTag     string
	PrometheusURL   string
	DraEnabled      bool
	ImagePullPolicy corev1.PullPolicy
}

// ComputeDesiredState produces all K8s resources that should exist for the given config.
// Mock pools are excluded — they are handled separately via Helm.
func ComputeDesiredState(config *topology.ClusterConfig, params ReconcileParams) []runtime.Object {
	var resources []runtime.Object

	// Sort pool names for deterministic output
	poolNames := make([]string, 0, len(config.NodePools))
	for name := range config.NodePools {
		poolNames = append(poolNames, name)
	}
	sort.Strings(poolNames)

	for _, poolName := range poolNames {
		pool := config.NodePools[poolName]
		if pool.Gpu.Backend != constants.BackendFake {
			continue
		}
		resources = append(resources, buildFakePoolResources(poolName, config, params)...)
	}

	return resources
}

func buildFakePoolResources(poolName string, config *topology.ClusterConfig, params ReconcileParams) []runtime.Object {
	dpImage := ResolveImage(config.Components, "devicePlugin", "kwok-gpu-device-plugin", params.DefaultRegistry, params.FallbackTag)
	seImage := ResolveImage(config.Components, "statusExporter", "status-exporter", params.DefaultRegistry, params.FallbackTag)
	pullPolicy := params.ImagePullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullAlways
	}

	resources := []runtime.Object{
		buildKwokDevicePluginDeployment(poolName, params.Namespace, dpImage, pullPolicy),
		buildKwokStatusExporterDeployment(poolName, params.Namespace, seImage, params.PrometheusURL, pullPolicy),
		buildKwokStatusExporterService(poolName, params.Namespace),
	}

	if params.DraEnabled {
		draImage := ResolveImage(config.Components, "kwokDraPlugin", "kwok-dra-plugin", params.DefaultRegistry, params.FallbackTag)
		resources = append(resources, buildKwokDraPluginDeployment(poolName, params.Namespace, draImage, pullPolicy))
	}

	return resources
}

