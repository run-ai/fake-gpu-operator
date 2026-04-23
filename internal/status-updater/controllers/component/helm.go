package component

import (
	"sort"
)

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
