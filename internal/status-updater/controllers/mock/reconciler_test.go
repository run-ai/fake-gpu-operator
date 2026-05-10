package mock

import (
	"context"
	"strings"
	"testing"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	commonprofile "github.com/run-ai/fake-gpu-operator/internal/common/profile"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func makeTopologyCM(t *testing.T, ns string, cfg *topology.ClusterConfig) *corev1.ConfigMap {
	t.Helper()
	body, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "topology", Namespace: ns},
		Data:       map[string]string{"topology.yml": string(body)},
	}
}

func makeProfileCM(name, ns string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    map[string]string{commonprofile.LabelGpuProfile: "true"},
		},
		Data: map[string]string{commonprofile.CmProfileKey: `system: { driver_version: "550" }`},
	}
}

func TestReconcile_Empty_NoOp(t *testing.T) {
	cfg := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools:        map[string]topology.NodePoolConfig{},
	}
	kube := fake.NewSimpleClientset(makeTopologyCM(t, "ns", cfg))
	r := NewReconciler(kube, ReconcileParams{Namespace: "ns"})
	require.NoError(t, r.Reconcile(context.Background()))
}

func TestReconcile_PoolAdded_CreatesResources(t *testing.T) {
	cfg := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools: map[string]topology.NodePoolConfig{
			"train": {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
		},
	}
	kube := fake.NewSimpleClientset(
		makeTopologyCM(t, "ns", cfg),
		makeProfileCM("gpu-profile-a100", "ns"),
	)
	r := NewReconciler(kube, ReconcileParams{
		Namespace: "ns", Image: "ghcr.io/nvidia/nvml-mock:v0.1.0",
	})
	require.NoError(t, r.Reconcile(context.Background()))

	ds, err := kube.AppsV1().DaemonSets("ns").Get(context.Background(), "nvml-mock-train", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, constants.LabelManagedByValue, ds.Labels[constants.LabelManagedBy])

	cm, err := kube.CoreV1().ConfigMaps("ns").Get(context.Background(), "nvml-mock-train", metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotEmpty(t, cm.Data["config.yaml"])
}

func TestReconcile_PoolRemoved_DeletesResources(t *testing.T) {
	cfg := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools:        map[string]topology.NodePoolConfig{}, // no pools
	}
	preexistingDS := BuildDaemonSet(BuildDaemonSetParams{
		Namespace: "ns", Pool: "old", NodePoolLabelKey: "k",
		Image: "img", ImagePullPolicy: corev1.PullAlways, ConfigHash: "h",
	})
	preexistingCM := BuildConfigMap("ns", "old", []byte("body"))

	kube := fake.NewSimpleClientset(
		makeTopologyCM(t, "ns", cfg),
		preexistingDS,
		preexistingCM,
	)
	r := NewReconciler(kube, ReconcileParams{Namespace: "ns"})
	require.NoError(t, r.Reconcile(context.Background()))

	_, err := kube.AppsV1().DaemonSets("ns").Get(context.Background(), "nvml-mock-old", metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err), "DaemonSet should have been deleted, got: %v", err)

	_, err = kube.CoreV1().ConfigMaps("ns").Get(context.Background(), "nvml-mock-old", metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err), "ConfigMap should have been deleted, got: %v", err)
}

func TestReconcile_OverrideChange_UpdatesConfigMapAndStampsDaemonSet(t *testing.T) {
	cfg1 := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools: map[string]topology.NodePoolConfig{
			"train": {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
		},
	}
	kube := fake.NewSimpleClientset(
		makeTopologyCM(t, "ns", cfg1),
		makeProfileCM("gpu-profile-a100", "ns"),
	)
	r := NewReconciler(kube, ReconcileParams{
		Namespace: "ns", Image: "img:t",
	})
	require.NoError(t, r.Reconcile(context.Background()))

	ds1, err := kube.AppsV1().DaemonSets("ns").Get(context.Background(), "nvml-mock-train", metav1.GetOptions{})
	require.NoError(t, err)
	hash1 := ds1.Spec.Template.Annotations[ConfigHashAnnotation]
	require.NotEmpty(t, hash1)

	// Override the driver_version. CM body must change → hash must change.
	cfg2 := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools: map[string]topology.NodePoolConfig{
			"train": {Gpu: topology.GpuConfig{
				Backend: "mock", Profile: "a100",
				Overrides: map[string]interface{}{"system": map[string]interface{}{"driver_version": "999"}},
			}},
		},
	}
	body, err := yaml.Marshal(cfg2)
	require.NoError(t, err)
	currentTopologyCM, err := kube.CoreV1().ConfigMaps("ns").Get(context.Background(), "topology", metav1.GetOptions{})
	require.NoError(t, err)
	currentTopologyCM.Data = map[string]string{"topology.yml": string(body)}
	_, err = kube.CoreV1().ConfigMaps("ns").Update(context.Background(), currentTopologyCM, metav1.UpdateOptions{})
	require.NoError(t, err)

	require.NoError(t, r.Reconcile(context.Background()))

	ds2, err := kube.AppsV1().DaemonSets("ns").Get(context.Background(), "nvml-mock-train", metav1.GetOptions{})
	require.NoError(t, err)
	hash2 := ds2.Spec.Template.Annotations[ConfigHashAnnotation]
	assert.NotEqual(t, hash1, hash2, "config-hash must change when override changes")

	cm, err := kube.CoreV1().ConfigMaps("ns").Get(context.Background(), "nvml-mock-train", metav1.GetOptions{})
	require.NoError(t, err)
	assert.True(t, strings.Contains(cm.Data["config.yaml"], "999"), "ConfigMap content should reflect the override")
}
