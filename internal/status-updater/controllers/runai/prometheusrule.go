package runai

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var prometheusRuleGVR = schema.GroupVersionResource{
	Group:    "monitoring.coreos.com",
	Version:  "v1",
	Resource: "prometheusrules",
}

const prometheusRuleName = "fake-gpu-operator-kwok-dcgm"

func buildKwokPrometheusRule(namespace string) *unstructured.Unstructured {
	labelReplaceExpr := func(metric string) string {
		return `label_replace(
  label_replace(
    label_replace(
      ` + metric + `,
      "pod_name", "$1", "exported_pod", "(.+)"
    ),
    "pod_namespace", "$1", "exported_namespace", "(.+)"
  ),
  "node", "$1", "Hostname", "(.+)"
)
* on(node) group_left(nodepool)
runai_node_nodepool_excluded`
	}

	rule := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "monitoring.coreos.com/v1",
			"kind":       "PrometheusRule",
			"metadata": map[string]interface{}{
				"name":      prometheusRuleName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"app":       "nvidia-dcgm-exporter",
					"component": "status-exporter-kwok",
				},
			},
			"spec": map[string]interface{}{
				"groups": []interface{}{
					map[string]interface{}{
						"name": "kwok-dcgm-metrics",
						"rules": []interface{}{
							map[string]interface{}{
								"record": "runai_dcgm_gpu_utilization",
								"expr":   labelReplaceExpr("DCGM_FI_DEV_GPU_UTIL"),
							},
							map[string]interface{}{
								"record": "runai_dcgm_gpu_used_mebibytes",
								"expr":   labelReplaceExpr("DCGM_FI_DEV_FB_USED"),
							},
							map[string]interface{}{
								"record": "runai_dcgm_gpu_total_mebibytes",
								"expr":   labelReplaceExpr("(DCGM_FI_DEV_FB_USED + DCGM_FI_DEV_FB_FREE)"),
							},
						},
					},
				},
			},
		},
	}

	return rule
}

// unstructuredNestedSlice is a helper to extract nested slices from unstructured objects.
func unstructuredNestedSlice(obj map[string]interface{}, fields ...string) ([]interface{}, bool, error) {
	return unstructured.NestedSlice(obj, fields...)
}
