package component

import (
	"context"
	"sort"
)

// HelmManager abstracts GPU Operator Helm lifecycle operations.
type HelmManager interface {
	// Sync ensures the GPU Operator Helm release matches the desired state.
	// If mockPools is empty, uninstalls the release. Otherwise installs or upgrades.
	Sync(ctx context.Context, mockPools []string, nodePoolLabelKey string, chartVersion string) error
}

// NoopHelmManager is used when GPU Operator management is disabled.
type NoopHelmManager struct{}

func (n *NoopHelmManager) Sync(_ context.Context, _ []string, _ string, _ string) error {
	return nil
}

// BuildGpuOperatorValues generates Helm values for the GPU Operator release
// that scopes its DaemonSets to the given mock pool nodes.
func BuildGpuOperatorValues(mockPools []string, nodePoolLabelKey string) map[string]interface{} {
	sort.Strings(mockPools)

	poolValues := make([]interface{}, len(mockPools))
	for i, p := range mockPools {
		poolValues[i] = p
	}

	affinity := map[string]interface{}{
		"nodeAffinity": map[string]interface{}{
			"requiredDuringSchedulingIgnoredDuringExecution": map[string]interface{}{
				"nodeSelectorTerms": []interface{}{
					map[string]interface{}{
						"matchExpressions": []interface{}{
							map[string]interface{}{
								"key":      nodePoolLabelKey,
								"operator": "In",
								"values":   poolValues,
							},
						},
					},
				},
			},
		},
	}

	return map[string]interface{}{
		"operator": map[string]interface{}{
			"defaultRuntime": "containerd",
		},
		"driver": map[string]interface{}{
			"enabled": false,
		},
		"toolkit": map[string]interface{}{
			"enabled": false,
		},
		"devicePlugin": map[string]interface{}{
			"affinity": affinity,
		},
		"dcgmExporter": map[string]interface{}{
			"affinity": affinity,
		},
		"daemonsets": map[string]interface{}{
			"labels": map[string]interface{}{
				"app.kubernetes.io/managed-by": "fake-gpu-operator",
			},
		},
	}
}
