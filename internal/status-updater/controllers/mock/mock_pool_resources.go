package mock

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// ReconcileParams holds the controller's runtime configuration.
type ReconcileParams struct {
	Namespace string
	// Image is the full nvml-mock image reference (e.g.
	// "ghcr.io/nvidia/nvml-mock:v0.1.0"), supplied via the NVML_MOCK_IMAGE
	// env var the chart plumbs from values.yaml's nvmlMock.image.repository
	// + tag. Every per-pool DaemonSet gets the same image — there's no
	// per-pool image override mechanism by design.
	Image           string
	ImagePullPolicy corev1.PullPolicy
}

// ComputeMockPoolResources walks all mock pools in the topology config and
// produces the per-pool ConfigMap + DaemonSet pairs the controller should
// ensure exist. Pools are iterated in sorted order so the returned slices
// are deterministic across calls.
func ComputeMockPoolResources(
	kube kubernetes.Interface,
	cfg *topology.ClusterConfig,
	params ReconcileParams,
) ([]*corev1.ConfigMap, []*appsv1.DaemonSet, error) {
	poolNames := make([]string, 0, len(cfg.NodePools))
	for name := range cfg.NodePools {
		poolNames = append(poolNames, name)
	}
	sort.Strings(poolNames)

	pullPolicy := params.ImagePullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullAlways
	}

	var configMaps []*corev1.ConfigMap
	var daemonSets []*appsv1.DaemonSet
	for _, name := range poolNames {
		pool := cfg.NodePools[name]
		if pool.Gpu.Backend != constants.BackendMock {
			continue
		}

		configYAML, spec, err := RenderConfig(kube, params.Namespace, pool.Gpu)
		if err != nil {
			return nil, nil, fmt.Errorf("rendering config for pool %q: %w", name, err)
		}

		configMaps = append(configMaps, BuildConfigMap(params.Namespace, name, configYAML))
		daemonSets = append(daemonSets, BuildDaemonSet(BuildDaemonSetParams{
			Namespace:        params.Namespace,
			Pool:             name,
			NodePoolLabelKey: cfg.NodePoolLabelKey,
			Image:            params.Image,
			ImagePullPolicy:  pullPolicy,
			GpuCount:         spec.GpuCount,
			DriverVersion:    spec.DriverVersion,
			ConfigHash:       configHash(configYAML),
		}))
	}
	return configMaps, daemonSets, nil
}

// configHash produces a hex SHA-256 of the rendered config YAML.
// Stamped on the DaemonSet pod template via the ConfigHashAnnotation so a CM
// content change forces a rolling restart.
func configHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
