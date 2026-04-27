package component

import (
	"fmt"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func managedLabels(component, pool string) map[string]string {
	return map[string]string{
		constants.LabelManagedBy: constants.LabelManagedByValue,
		constants.LabelComponent: component,
		constants.LabelPool:      pool,
	}
}

func resourceName(base, pool string) string {
	return fmt.Sprintf("%s-%s", base, pool)
}

func buildKwokDevicePluginDeployment(pool, namespace, image string, pullPolicy corev1.PullPolicy) *appsv1.Deployment {
	name := resourceName("kwok-gpu-device-plugin", pool)
	labels := managedLabels(constants.ComponentKwokDevicePlugin, pool)
	labels["app"] = name

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":              name,
					constants.LabelPool: pool,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "kwok-gpu-device-plugin",
							Image:           image,
							ImagePullPolicy: pullPolicy,
							Env: []corev1.EnvVar{
								{Name: "TOPOLOGY_CM_NAME", Value: "topology"},
								{Name: "TOPOLOGY_CM_NAMESPACE", Value: namespace},
								{Name: "FAKE_GPU_OPERATOR_NAMESPACE", Value: namespace},
								{Name: "NODE_POOL", Value: pool},
							},
						},
					},
					RestartPolicy:      corev1.RestartPolicyAlways,
					ServiceAccountName: "kwok-gpu-device-plugin",
					ImagePullSecrets: []corev1.LocalObjectReference{
						{Name: "gcr-secret"},
					},
				},
			},
		},
	}
}

func buildKwokStatusExporterDeployment(pool, namespace, image, prometheusURL string, pullPolicy corev1.PullPolicy) *appsv1.Deployment {
	name := resourceName("kwok-status-exporter", pool)
	labels := managedLabels(constants.ComponentKwokStatusExporter, pool)
	labels["app"] = name

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":              name,
					constants.LabelPool: pool,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "kwok-status-exporter",
							Image:           image,
							ImagePullPolicy: pullPolicy,
							Command:         []string{"/usr/local/bin/status-exporter-kwok"},
							Env: []corev1.EnvVar{
								{Name: "TOPOLOGY_CM_NAME", Value: "topology"},
								{Name: "TOPOLOGY_CM_NAMESPACE", Value: namespace},
								{Name: "PROMETHEUS_URL", Value: prometheusURL},
								{Name: "NODE_POOL", Value: pool},
							},
							Ports: []corev1.ContainerPort{
								{ContainerPort: 9400, Name: "http"},
							},
						},
					},
					RestartPolicy:      corev1.RestartPolicyAlways,
					ServiceAccountName: "status-exporter",
					ImagePullSecrets: []corev1.LocalObjectReference{
						{Name: "gcr-secret"},
					},
				},
			},
		},
	}
}

func buildKwokDraPluginDeployment(pool, namespace, image string, pullPolicy corev1.PullPolicy) *appsv1.Deployment {
	name := resourceName("kwok-dra-plugin", pool)
	labels := managedLabels(constants.ComponentKwokDraPlugin, pool)
	labels["app"] = name

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":              name,
					constants.LabelPool: pool,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            "kwok-dra-plugin",
							Image:           image,
							ImagePullPolicy: pullPolicy,
							Env: []corev1.EnvVar{
								{Name: "TOPOLOGY_CM_NAME", Value: "topology"},
								{Name: "TOPOLOGY_CM_NAMESPACE", Value: namespace},
								{Name: "FAKE_GPU_OPERATOR_NAMESPACE", Value: namespace},
								{Name: "NODE_POOL", Value: pool},
							},
						},
					},
					RestartPolicy:      corev1.RestartPolicyAlways,
					ServiceAccountName: "kwok-dra-plugin",
					ImagePullSecrets: []corev1.LocalObjectReference{
						{Name: "gcr-secret"},
					},
				},
			},
		},
	}
}

func buildKwokStatusExporterService(pool, namespace string) *corev1.Service {
	name := resourceName("kwok-status-exporter", pool)
	labels := managedLabels(constants.ComponentKwokStatusExporter, pool)
	labels["app"] = name

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"prometheus.io/scrape": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "gpu-metrics", Port: 9400, Protocol: corev1.ProtocolTCP},
			},
			Selector: map[string]string{
				"app":              name,
				constants.LabelPool: pool,
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}
