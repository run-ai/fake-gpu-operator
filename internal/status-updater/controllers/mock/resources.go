package mock

import (
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	// ConfigHashAnnotation is stamped on the DaemonSet pod template so a
	// ConfigMap content change forces a pod rollout.
	ConfigHashAnnotation = "fake-gpu-operator/config-hash"

	// configKey is the data key in the per-pool ConfigMap. Mirrors
	// nvml-mock's chart, which mounts the file at /config/config.yaml.
	configKey = "config.yaml"
)

// resourceName produces the per-pool resource name shared by the DaemonSet
// and the ConfigMap (also used as the diff key).
func resourceName(pool string) string {
	return "nvml-mock-" + pool
}

// managedLabels returns the label set every controller-owned resource carries.
func managedLabels(pool string) map[string]string {
	return map[string]string{
		constants.LabelManagedBy: constants.LabelManagedByValue,
		constants.LabelComponent: constants.ComponentNvmlMock,
		constants.LabelPool:      pool,
	}
}

// BuildConfigMap produces the per-pool nvml-mock ConfigMap whose `config.yaml`
// key is mounted into the DaemonSet at /config/config.yaml.
func BuildConfigMap(namespace, pool string, configYAML []byte) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName(pool),
			Namespace: namespace,
			Labels:    managedLabels(pool),
		},
		Data: map[string]string{
			configKey: string(configYAML),
		},
	}
}

// BuildDaemonSetParams captures everything BuildDaemonSet needs.
type BuildDaemonSetParams struct {
	Namespace        string
	Pool             string
	NodePoolLabelKey string
	Image            string
	ImagePullPolicy  corev1.PullPolicy
	// ConfigHash is the SHA of the ConfigMap's config.yaml content; stamped
	// on the pod template so changes force a rolling restart.
	ConfigHash string
}

// BuildDaemonSet produces the per-pool nvml-mock DaemonSet. Mirrors upstream
// nvml-mock v0.1.0 templates/daemonset.yaml. When bumping nvml-mock, re-read
// upstream's daemonset.yaml and reconcile this builder.
func BuildDaemonSet(p BuildDaemonSetParams) *appsv1.DaemonSet {
	labels := managedLabels(p.Pool)
	name := resourceName(p.Pool)

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: p.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: map[string]string{ConfigHashAnnotation: p.ConfigHash},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "nvml-mock",
					NodeSelector:       map[string]string{p.NodePoolLabelKey: p.Pool},
					Containers: []corev1.Container{{
						Name:            "nvml-mock",
						Image:           p.Image,
						ImagePullPolicy: p.ImagePullPolicy,
						SecurityContext: &corev1.SecurityContext{Privileged: ptr.To(true)},
						Command:         []string{"/scripts/entrypoint.sh"},
						Lifecycle: &corev1.Lifecycle{
							PreStop: &corev1.LifecycleHandler{
								Exec: &corev1.ExecAction{Command: []string{"/scripts/cleanup.sh"}},
							},
						},
						Env: []corev1.EnvVar{
							{
								Name: "GPU_COUNT",
								ValueFrom: &corev1.EnvVarSource{
									ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: name},
										Key:                  "GPU_COUNT",
										Optional:             ptr.To(true),
									},
								},
							},
							{
								Name: "DRIVER_VERSION",
								ValueFrom: &corev1.EnvVarSource{
									ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: name},
										Key:                  "DRIVER_VERSION",
										Optional:             ptr.To(true),
									},
								},
							},
							{
								Name:      "NODE_NAME",
								ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}},
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "host-nvml-mock", MountPath: "/host/var/lib/nvml-mock"},
							{Name: "gpu-config", MountPath: "/config"},
							{Name: "host-cdi", MountPath: "/host/var/run/cdi"},
							{Name: "host-run-nvidia", MountPath: "/host/run/nvidia"},
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: "host-nvml-mock",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/lib/nvml-mock",
									Type: ptr.To(corev1.HostPathDirectoryOrCreate),
								},
							},
						},
						{
							Name: "gpu-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: name},
								},
							},
						},
						{
							Name: "host-cdi",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/run/cdi",
									Type: ptr.To(corev1.HostPathDirectoryOrCreate),
								},
							},
						},
						{
							Name: "host-run-nvidia",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/run/nvidia",
									Type: ptr.To(corev1.HostPathDirectoryOrCreate),
								},
							},
						},
					},
				},
			},
		},
	}
}
