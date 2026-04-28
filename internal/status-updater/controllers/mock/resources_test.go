package mock

import (
	"testing"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestBuildConfigMap(t *testing.T) {
	cm := BuildConfigMap("ns", "training", []byte("driver_version: \"550\""))

	assert.Equal(t, "nvml-mock-training", cm.Name)
	assert.Equal(t, "ns", cm.Namespace)
	assert.Equal(t, constants.LabelManagedByValue, cm.Labels[constants.LabelManagedBy])
	assert.Equal(t, constants.ComponentNvmlMock, cm.Labels[constants.LabelComponent])
	assert.Equal(t, "training", cm.Labels[constants.LabelPool])
	assert.Equal(t, "driver_version: \"550\"", cm.Data["config.yaml"])
}

func TestBuildDaemonSet_Shape(t *testing.T) {
	ds := BuildDaemonSet(BuildDaemonSetParams{
		Namespace:        "ns",
		Pool:             "training",
		NodePoolLabelKey: "run.ai/simulated-gpu-node-pool",
		Image:            "ghcr.io/nvidia/nvml-mock:v0.1.0",
		ImagePullPolicy:  corev1.PullIfNotPresent,
		ConfigHash:       "abc123",
	})

	require.Equal(t, "nvml-mock-training", ds.Name)
	assert.Equal(t, "ns", ds.Namespace)
	assert.Equal(t, constants.LabelManagedByValue, ds.Labels[constants.LabelManagedBy])
	assert.Equal(t, constants.ComponentNvmlMock, ds.Labels[constants.LabelComponent])
	assert.Equal(t, "training", ds.Labels[constants.LabelPool])

	pod := ds.Spec.Template
	assert.Equal(t, "training", pod.Spec.NodeSelector["run.ai/simulated-gpu-node-pool"])
	assert.Equal(t, "nvml-mock", pod.Spec.ServiceAccountName)
	assert.Equal(t, "abc123", pod.Annotations[ConfigHashAnnotation])

	require.Len(t, pod.Spec.Containers, 1)
	c := pod.Spec.Containers[0]
	assert.Equal(t, "nvml-mock", c.Name)
	assert.Equal(t, "ghcr.io/nvidia/nvml-mock:v0.1.0", c.Image)
	assert.Equal(t, corev1.PullIfNotPresent, c.ImagePullPolicy)
	require.NotNil(t, c.SecurityContext)
	require.NotNil(t, c.SecurityContext.Privileged)
	assert.True(t, *c.SecurityContext.Privileged)

	envNames := map[string]bool{}
	for _, e := range c.Env {
		envNames[e.Name] = true
	}
	assert.True(t, envNames["GPU_COUNT"])
	assert.True(t, envNames["DRIVER_VERSION"])
	assert.True(t, envNames["NODE_NAME"])

	mountPaths := map[string]bool{}
	for _, m := range c.VolumeMounts {
		mountPaths[m.MountPath] = true
	}
	assert.True(t, mountPaths["/host/var/lib/nvml-mock"])
	assert.True(t, mountPaths["/config"])
	assert.True(t, mountPaths["/host/var/run/cdi"])
	assert.True(t, mountPaths["/host/run/nvidia"])
}

func TestBuildDaemonSet_ConfigMapVolumePointsAtPerPoolCM(t *testing.T) {
	ds := BuildDaemonSet(BuildDaemonSetParams{
		Namespace: "ns", Pool: "training",
		NodePoolLabelKey: "k", Image: "img", ImagePullPolicy: corev1.PullAlways,
		ConfigHash: "x",
	})
	for _, v := range ds.Spec.Template.Spec.Volumes {
		if v.Name == "gpu-config" {
			require.NotNil(t, v.ConfigMap)
			assert.Equal(t, "nvml-mock-training", v.ConfigMap.Name)
			return
		}
	}
	t.Fatal("gpu-config volume not found")
}
