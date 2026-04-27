package node

import (
	"context"
	"testing"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestLabelNode_Matrix(t *testing.T) {
	const poolKey = "run.ai/simulated-gpu-node-pool"

	clusterConfig := &topology.ClusterConfig{
		NodePoolLabelKey: poolKey,
		NodePools: map[string]topology.NodePoolConfig{
			"fake-pool": {Gpu: topology.GpuConfig{Backend: constants.BackendFake}},
			"mock-pool": {Gpu: topology.GpuConfig{Backend: constants.BackendMock}},
		},
	}

	cases := []struct {
		name              string
		nodeLabels        map[string]string
		annotations       map[string]string
		wantDevicePlugin  bool
		wantDraPlugin     bool
		wantComputeDomain bool
	}{
		{
			name:        "KWOK + fake pool: dcgm only",
			nodeLabels:  map[string]string{poolKey: "fake-pool"},
			annotations: map[string]string{constants.AnnotationKwokNode: "fake"},
		},
		{
			name:              "Real Linux + mock pool: all four",
			nodeLabels:        map[string]string{poolKey: "mock-pool"},
			wantDevicePlugin:  true,
			wantDraPlugin:     true,
			wantComputeDomain: true,
		},
		{
			name:       "Real Linux + fake pool: dcgm only",
			nodeLabels: map[string]string{poolKey: "fake-pool"},
		},
		{
			name:       "Real Linux + no pool match in CM: dcgm only",
			nodeLabels: map[string]string{poolKey: "unknown-pool"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "n1",
					Labels:      tc.nodeLabels,
					Annotations: tc.annotations,
				},
			}
			kubeClient := fake.NewSimpleClientset(node)
			h := NewNodeHandler(kubeClient, clusterConfig, false)

			require.NoError(t, h.labelNode(node))

			got, err := kubeClient.CoreV1().Nodes().Get(context.TODO(), "n1", metav1.GetOptions{})
			require.NoError(t, err)

			// dcgm-exporter is always applied
			assert.Equal(t, "true", got.Labels[dcgmExporterLabelKey],
				"dcgm-exporter label always applied")

			assertLabel(t, got.Labels, devicePluginLabelKey, tc.wantDevicePlugin)
			assertLabel(t, got.Labels, draPluginGpuLabelKey, tc.wantDraPlugin)
			assertLabel(t, got.Labels, computeDomainDevicePluginLabelKey, tc.wantComputeDomain)
		})
	}
}

func assertLabel(t *testing.T, labels map[string]string, key string, want bool) {
	t.Helper()
	if want {
		assert.Equal(t, "true", labels[key], "expected label %q=true", key)
	} else {
		assert.NotContains(t, labels, key, "expected label %q absent", key)
	}
}
