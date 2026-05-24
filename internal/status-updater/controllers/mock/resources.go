package mock

import (
	"fmt"

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
	// nvml-mock's chart, which mounts the file at /etc/nvml-mock/config.yaml.
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
	// GpuCount and DriverVersion are extracted from the resolved profile and
	// stamped onto the container as literal env vars. nvml-mock's setup.sh
	// requires both as integers/strings; the upstream chart sets them the same
	// way (no ConfigMap indirection) since the ConfigMap only carries the
	// profile YAML at /config/config.yaml.
	GpuCount      int
	DriverVersion string
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
						// Background upstream's entrypoint.sh, wait for its setup.sh to
						// signal completion (creating /run/nvidia/driver as a symlink
						// near its end), touch /run/nvidia/validations/toolkit-ready,
						// then `wait` to inherit entrypoint.sh's sleep-infinity. Without
						// the marker, gpu-operator operand pods (device-plugin, GFD)
						// block at Init:0/1 forever and `nvidia.com/gpu` never becomes
						// allocatable. We do this in the container's own process tree
						// (rather than a postStart hook) to avoid kubelet→containerd
						// gRPC failures during the toolkit DS's containerd reload.
						// Tracked upstream at NVIDIA/k8s-test-infra#346; drop this
						// wrapper when the marker write lands in nvml-mock's setup.sh.
						Command: []string{"/bin/sh", "-c"},
						Args: []string{
							"/scripts/entrypoint.sh & ENTRY=$!; " +
								"while ! [ -L /host/run/nvidia/driver ] && kill -0 $ENTRY 2>/dev/null; do sleep 1; done; " +
								"[ -L /host/run/nvidia/driver ] && mkdir -p /host/run/nvidia/validations && touch /host/run/nvidia/validations/toolkit-ready; " +
								"wait $ENTRY",
						},
						Lifecycle: &corev1.Lifecycle{
							// Deliberately does NOT remove the toolkit-ready marker:
							// on DaemonSet recreate the old pod's preStop races with
							// the new pod's setup wrapper (both touch the same host
							// path) and an `rm` here can blow away the marker the new
							// pod just wrote. The marker is sticky-positive — leaving
							// it on shutdown is harmless because operand pods that
							// scheduled while we were running already passed their
							// init poll.
							PreStop: &corev1.LifecycleHandler{
								Exec: &corev1.ExecAction{Command: []string{"/scripts/cleanup.sh"}},
							},
						},
						Env: []corev1.EnvVar{
							{Name: "GPU_COUNT", Value: fmt.Sprintf("%d", p.GpuCount)},
							{Name: "DRIVER_VERSION", Value: p.DriverVersion},
							{
								Name:      "NODE_NAME",
								ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}},
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "host-nvml-mock", MountPath: "/host/var/lib/nvml-mock"},
							{Name: "gpu-config", MountPath: "/etc/nvml-mock"},
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
