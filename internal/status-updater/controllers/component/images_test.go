package component

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

func TestComponent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Component Controller Suite")
}

var _ = Describe("ResolveImage", func() {
	const (
		defaultRegistry = "ghcr.io/run-ai/fake-gpu-operator"
		fallbackTag     = "0.3.0"
	)

	It("should use per-component full image when set", func() {
		components := &topology.ComponentsConfig{
			ImageTag:      "0.5.0",
			ImageRegistry: "gcr.io/custom",
			DevicePlugin:  &topology.ComponentImageConfig{Image: "my-registry/dp:1.0.0"},
		}
		result := ResolveImage(components, "devicePlugin", "kwok-gpu-device-plugin", defaultRegistry, fallbackTag)
		Expect(result).To(Equal("my-registry/dp:1.0.0"))
	})

	It("should use per-component imageTag with global registry", func() {
		components := &topology.ComponentsConfig{
			ImageRegistry:  "gcr.io/custom",
			StatusExporter: &topology.ComponentImageConfig{ImageTag: "2.0.0"},
		}
		result := ResolveImage(components, "statusExporter", "status-exporter", defaultRegistry, fallbackTag)
		Expect(result).To(Equal("gcr.io/custom/status-exporter:2.0.0"))
	})

	It("should use global imageTag + global imageRegistry", func() {
		components := &topology.ComponentsConfig{
			ImageTag:      "0.5.0",
			ImageRegistry: "gcr.io/custom",
		}
		result := ResolveImage(components, "devicePlugin", "kwok-gpu-device-plugin", defaultRegistry, fallbackTag)
		Expect(result).To(Equal("gcr.io/custom/kwok-gpu-device-plugin:0.5.0"))
	})

	It("should use global imageTag + default registry when no global registry", func() {
		components := &topology.ComponentsConfig{
			ImageTag: "0.5.0",
		}
		result := ResolveImage(components, "devicePlugin", "kwok-gpu-device-plugin", defaultRegistry, fallbackTag)
		Expect(result).To(Equal("ghcr.io/run-ai/fake-gpu-operator/kwok-gpu-device-plugin:0.5.0"))
	})

	It("should fall back to operator version when no components config", func() {
		result := ResolveImage(nil, "devicePlugin", "kwok-gpu-device-plugin", defaultRegistry, fallbackTag)
		Expect(result).To(Equal("ghcr.io/run-ai/fake-gpu-operator/kwok-gpu-device-plugin:0.3.0"))
	})

	It("should fall back to operator version when components has no relevant overrides", func() {
		components := &topology.ComponentsConfig{}
		result := ResolveImage(components, "devicePlugin", "kwok-gpu-device-plugin", defaultRegistry, fallbackTag)
		Expect(result).To(Equal("ghcr.io/run-ai/fake-gpu-operator/kwok-gpu-device-plugin:0.3.0"))
	})
})
