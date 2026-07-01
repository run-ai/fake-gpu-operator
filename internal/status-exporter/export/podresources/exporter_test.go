package podresources

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

func TestExporter_Reconcile(t *testing.T) {
	// topology CM (cluster config with a 2-zone numa pool) + node + workload pod.
	clusterCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "topology", Namespace: "gpu-operator"},
		Data: map[string]string{"topology.yml": `
nodePoolLabelKey: run.ai/simulated-gpu-node-pool
nodePools:
  default:
    gpu: {backend: fake}
    numa: {zones: 2}
`},
	}
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{"run.ai/simulated-gpu-node-pool": "default"}},
		Status:     corev1.NodeStatus{Allocatable: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("16"), corev1.ResourceMemory: resource.MustParse("64Gi")}},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "team"},
		Spec: corev1.PodSpec{NodeName: "n1", Containers: []corev1.Container{{
			Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("4"), corev1.ResourceMemory: resource.MustParse("8Gi")}},
		}}},
	}
	kube := fake.NewSimpleClientset(clusterCM, node, pod)

	viper.Set(constants.EnvNodeName, "n1")
	viper.Set(constants.EnvTopologyCmName, "topology")
	viper.Set(constants.EnvTopologyCmNamespace, "gpu-operator")

	root := t.TempDir()
	e := NewExporter(nil, kube)
	e.sysfsRoot = root // test override

	nt := &topology.NodeTopology{Gpus: []topology.GpuDetails{
		{ID: "g0", Status: topology.GpuStatus{AllocatedBy: topology.ContainerDetails{Namespace: "team", Pod: "p1"}}},
	}}
	require.NoError(t, e.reconcile(nt))

	// Snapshot has the pod with a single zone-0 GPU device.
	resp := e.server.snapshot()
	require.Len(t, resp, 1)
	assert.Equal(t, "p1", resp[0].Name)

	// sysfs tree rendered.
	_, err := os.Stat(filepath.Join(root, "devices/system/node/node0/cpulist"))
	assert.NoError(t, err)
}
