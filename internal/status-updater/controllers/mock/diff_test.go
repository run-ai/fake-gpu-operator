package mock

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newDS(name, image, configHash string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{ConfigHashAnnotation: configHash},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Image: image}},
				},
			},
		},
	}
}

func newCM(name, body string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Data:       map[string]string{"config.yaml": body},
	}
}

func TestDiffDaemonSets_Create(t *testing.T) {
	desired := []*appsv1.DaemonSet{newDS("nvml-mock-a", "img:1", "h1")}
	d := DiffDaemonSets(desired, nil)
	require.Len(t, d.ToCreate, 1)
	assert.Equal(t, "nvml-mock-a", d.ToCreate[0].Name)
}

func TestDiffDaemonSets_NoOp(t *testing.T) {
	desired := []*appsv1.DaemonSet{newDS("nvml-mock-a", "img:1", "h1")}
	actual := []appsv1.DaemonSet{*newDS("nvml-mock-a", "img:1", "h1")}
	d := DiffDaemonSets(desired, actual)
	assert.Empty(t, d.ToCreate)
	assert.Empty(t, d.ToUpdate)
	assert.Empty(t, d.ToDelete)
}

func TestDiffDaemonSets_UpdateOnImageChange(t *testing.T) {
	desired := []*appsv1.DaemonSet{newDS("nvml-mock-a", "img:2", "h1")}
	existing := newDS("nvml-mock-a", "img:1", "h1")
	existing.ResourceVersion = "42"
	d := DiffDaemonSets(desired, []appsv1.DaemonSet{*existing})
	require.Len(t, d.ToUpdate, 1)
	assert.Equal(t, "42", d.ToUpdate[0].ResourceVersion, "ResourceVersion copied for optimistic concurrency")
}

func TestDiffDaemonSets_UpdateOnConfigHashChange(t *testing.T) {
	desired := []*appsv1.DaemonSet{newDS("nvml-mock-a", "img:1", "h2")}
	existing := newDS("nvml-mock-a", "img:1", "h1")
	existing.ResourceVersion = "7"
	d := DiffDaemonSets(desired, []appsv1.DaemonSet{*existing})
	require.Len(t, d.ToUpdate, 1)
	assert.Equal(t, "h2", d.ToUpdate[0].Spec.Template.Annotations[ConfigHashAnnotation])
}

func TestDiffDaemonSets_UpdateOnCommandChange(t *testing.T) {
	desired := []*appsv1.DaemonSet{newDS("nvml-mock-a", "img:1", "h1")}
	desired[0].Spec.Template.Spec.Containers[0].Command = []string{"/bin/sh", "-c"}
	desired[0].Spec.Template.Spec.Containers[0].Args = []string{"/scripts/setup.sh && touch /marker"}
	existing := newDS("nvml-mock-a", "img:1", "h1")
	existing.Spec.Template.Spec.Containers[0].Command = []string{"/scripts/entrypoint.sh"}
	existing.ResourceVersion = "9"
	d := DiffDaemonSets(desired, []appsv1.DaemonSet{*existing})
	require.Len(t, d.ToUpdate, 1, "Command/Args drift must trigger Update — covers the status-updater-upgrade case")
}

func TestDiffDaemonSets_UpdateOnLifecycleChange(t *testing.T) {
	desired := []*appsv1.DaemonSet{newDS("nvml-mock-a", "img:1", "h1")}
	desired[0].Spec.Template.Spec.Containers[0].Lifecycle = &corev1.Lifecycle{
		PostStart: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{Command: []string{"/bin/sh", "-c", "touch /marker"}},
		},
	}
	existing := newDS("nvml-mock-a", "img:1", "h1")
	existing.Spec.Template.Spec.Containers[0].Lifecycle = nil
	existing.ResourceVersion = "11"
	d := DiffDaemonSets(desired, []appsv1.DaemonSet{*existing})
	require.Len(t, d.ToUpdate, 1, "Lifecycle drift must trigger Update — covers the postStart hook addition")
}

func TestDiffDaemonSets_Delete(t *testing.T) {
	actual := []appsv1.DaemonSet{*newDS("nvml-mock-old", "img:1", "h1")}
	d := DiffDaemonSets(nil, actual)
	require.Len(t, d.ToDelete, 1)
	assert.Equal(t, "nvml-mock-old", d.ToDelete[0].Name)
}

func TestDiffConfigMaps_Create(t *testing.T) {
	desired := []*corev1.ConfigMap{newCM("nvml-mock-a", "body1")}
	d := DiffConfigMaps(desired, nil)
	require.Len(t, d.ToCreate, 1)
}

func TestDiffConfigMaps_UpdateOnDataChange(t *testing.T) {
	desired := []*corev1.ConfigMap{newCM("nvml-mock-a", "body2")}
	existing := newCM("nvml-mock-a", "body1")
	existing.ResourceVersion = "13"
	d := DiffConfigMaps(desired, []corev1.ConfigMap{*existing})
	require.Len(t, d.ToUpdate, 1)
	assert.Equal(t, "13", d.ToUpdate[0].ResourceVersion)
	assert.Equal(t, "body2", d.ToUpdate[0].Data["config.yaml"])
}

func TestDiffConfigMaps_NoOp(t *testing.T) {
	desired := []*corev1.ConfigMap{newCM("nvml-mock-a", "body1")}
	actual := []corev1.ConfigMap{*newCM("nvml-mock-a", "body1")}
	d := DiffConfigMaps(desired, actual)
	assert.Empty(t, d.ToCreate)
	assert.Empty(t, d.ToUpdate)
	assert.Empty(t, d.ToDelete)
}

func TestDiffConfigMaps_Delete(t *testing.T) {
	actual := []corev1.ConfigMap{*newCM("nvml-mock-old", "body1")}
	d := DiffConfigMaps(nil, actual)
	require.Len(t, d.ToDelete, 1)
}
