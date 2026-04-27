package component

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("ComputeDesiredState", func() {
	var params ReconcileParams

	BeforeEach(func() {
		params = ReconcileParams{
			Namespace:       "fake-gpu-operator",
			DefaultRegistry: "ghcr.io/run-ai/fake-gpu-operator",
			FallbackTag:     "0.5.0",
			PrometheusURL:   "http://prometheus:9090",
			DraEnabled:      true,
		}
	})

	It("should produce deployments and service for a fake pool", func() {
		config := &topology.ClusterConfig{
			NodePools: map[string]topology.NodePoolConfig{
				"default": {Gpu: topology.GpuConfig{Backend: "fake"}},
			},
		}
		resources := ComputeDesiredState(config, params)
		names := resourceNames(resources)
		Expect(names).To(ContainElement("kwok-gpu-device-plugin-default"))
		Expect(names).To(ContainElement("kwok-status-exporter-default"))
		Expect(names).To(ContainElement("kwok-dra-plugin-default"))
	})

	It("should not produce DRA plugin when DRA is disabled", func() {
		params.DraEnabled = false
		config := &topology.ClusterConfig{
			NodePools: map[string]topology.NodePoolConfig{
				"default": {Gpu: topology.GpuConfig{Backend: "fake"}},
			},
		}
		resources := ComputeDesiredState(config, params)
		names := resourceNames(resources)
		Expect(names).To(ContainElement("kwok-gpu-device-plugin-default"))
		Expect(names).To(ContainElement("kwok-status-exporter-default"))
		Expect(names).ToNot(ContainElement("kwok-dra-plugin-default"))
	})

	It("should produce no resources for mock pools", func() {
		config := &topology.ClusterConfig{
			NodePools: map[string]topology.NodePoolConfig{
				"training": {Gpu: topology.GpuConfig{Backend: "mock"}},
			},
		}
		resources := ComputeDesiredState(config, params)
		Expect(resources).To(BeEmpty())
	})

	It("should produce resources for multiple fake pools", func() {
		config := &topology.ClusterConfig{
			NodePools: map[string]topology.NodePoolConfig{
				"default": {Gpu: topology.GpuConfig{Backend: "fake"}},
				"highend": {Gpu: topology.GpuConfig{Backend: "fake"}},
			},
		}
		resources := ComputeDesiredState(config, params)
		names := resourceNames(resources)
		Expect(names).To(ContainElement("kwok-gpu-device-plugin-default"))
		Expect(names).To(ContainElement("kwok-gpu-device-plugin-highend"))
		Expect(names).To(ContainElement("kwok-status-exporter-default"))
		Expect(names).To(ContainElement("kwok-status-exporter-highend"))
	})

	It("should use per-component image override", func() {
		config := &topology.ClusterConfig{
			NodePools: map[string]topology.NodePoolConfig{
				"default": {Gpu: topology.GpuConfig{Backend: "fake"}},
			},
			Components: &topology.ComponentsConfig{
				DevicePlugin: &topology.ComponentImageConfig{Image: "custom/dp:1.0"},
			},
		}
		resources := ComputeDesiredState(config, params)
		for _, r := range resources {
			if dep, ok := r.(*appsv1.Deployment); ok {
				if dep.Labels[constants.LabelComponent] == constants.ComponentKwokDevicePlugin {
					Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal("custom/dp:1.0"))
				}
			}
		}
	})
})

func resourceNames(objs []runtime.Object) []string {
	var names []string
	for _, obj := range objs {
		switch o := obj.(type) {
		case *appsv1.Deployment:
			names = append(names, o.Name)
		case *corev1.Service:
			names = append(names, o.Name)
		}
	}
	return names
}
