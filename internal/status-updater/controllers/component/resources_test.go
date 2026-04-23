package component

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("BuildPoolResources", func() {
	const (
		namespace = "fake-gpu-operator"
		poolName  = "default"
		image     = "ghcr.io/run-ai/fake-gpu-operator/kwok-gpu-device-plugin:0.5.0"
	)

	It("should build kwok-gpu-device-plugin deployment for a pool", func() {
		dep := buildKwokDevicePluginDeployment(poolName, namespace, image, corev1.PullAlways)
		Expect(dep.Name).To(Equal("kwok-gpu-device-plugin-default"))
		Expect(dep.Namespace).To(Equal(namespace))
		Expect(dep.Labels[constants.LabelManagedBy]).To(Equal(constants.LabelManagedByValue))
		Expect(dep.Labels[constants.LabelComponent]).To(Equal(constants.ComponentKwokDevicePlugin))
		Expect(dep.Labels[constants.LabelPool]).To(Equal(poolName))
		Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal(image))

		envMap := envVarsToMap(dep.Spec.Template.Spec.Containers[0].Env)
		Expect(envMap["TOPOLOGY_CM_NAME"]).To(Equal("topology"))
		Expect(envMap["TOPOLOGY_CM_NAMESPACE"]).To(Equal(namespace))
		Expect(envMap["FAKE_GPU_OPERATOR_NAMESPACE"]).To(Equal(namespace))
		Expect(envMap["NODE_POOL"]).To(Equal(poolName))
	})

	It("should build kwok-status-exporter deployment for a pool", func() {
		dep := buildKwokStatusExporterDeployment(poolName, namespace,
			"ghcr.io/run-ai/fake-gpu-operator/status-exporter:0.5.0",
			"http://prometheus:9090", corev1.PullAlways)
		Expect(dep.Name).To(Equal("kwok-status-exporter-default"))
		Expect(dep.Spec.Template.Spec.Containers[0].Command).To(Equal([]string{"/usr/local/bin/status-exporter-kwok"}))

		envMap := envVarsToMap(dep.Spec.Template.Spec.Containers[0].Env)
		Expect(envMap["PROMETHEUS_URL"]).To(Equal("http://prometheus:9090"))
	})

	It("should build kwok-dra-plugin deployment for a pool", func() {
		dep := buildKwokDraPluginDeployment(poolName, namespace,
			"ghcr.io/run-ai/fake-gpu-operator/kwok-dra-plugin:0.5.0", corev1.PullAlways)
		Expect(dep.Name).To(Equal("kwok-dra-plugin-default"))
	})

	It("should use pool name in resource names and labels", func() {
		dep := buildKwokDevicePluginDeployment("highend", namespace, image, corev1.PullAlways)
		Expect(dep.Name).To(Equal("kwok-gpu-device-plugin-highend"))
		Expect(dep.Labels[constants.LabelPool]).To(Equal("highend"))
		Expect(dep.Spec.Selector.MatchLabels[constants.LabelPool]).To(Equal("highend"))
	})

	It("should build kwok-status-exporter service for a pool", func() {
		svc := buildKwokStatusExporterService(poolName, namespace)
		Expect(svc.Name).To(Equal("kwok-status-exporter-default"))
		Expect(svc.Labels[constants.LabelManagedBy]).To(Equal(constants.LabelManagedByValue))
		Expect(svc.Annotations["prometheus.io/scrape"]).To(Equal("true"))
		Expect(svc.Spec.Ports[0].Port).To(Equal(int32(9400)))
	})
})

func envVarsToMap(envs []corev1.EnvVar) map[string]string {
	m := make(map[string]string)
	for _, e := range envs {
		m[e.Name] = e.Value
	}
	return m
}
