package component

import (
	"fmt"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

// ResolveImage determines the full container image reference for a component.
// Resolution order (highest priority first):
//  1. Per-component Image (full ref)
//  2. Per-component ImageTag + effective registry + componentName
//  3. Global ImageTag + effective registry + componentName
//  4. defaultRegistry + componentName + fallbackTag (operator's own version)
func ResolveImage(
	components *topology.ComponentsConfig,
	componentKey string,
	componentName string,
	defaultRegistry string,
	fallbackTag string,
) string {
	if components != nil {
		compConfig := getComponentConfig(components, componentKey)

		// Priority 1: per-component full image
		if compConfig != nil && compConfig.Image != "" {
			return compConfig.Image
		}

		registry := defaultRegistry
		if components.ImageRegistry != "" {
			registry = components.ImageRegistry
		}

		// Priority 2: per-component imageTag
		if compConfig != nil && compConfig.ImageTag != "" {
			return fmt.Sprintf("%s/%s:%s", registry, componentName, compConfig.ImageTag)
		}

		// Priority 3: global imageTag
		if components.ImageTag != "" {
			return fmt.Sprintf("%s/%s:%s", registry, componentName, components.ImageTag)
		}
	}

	// Priority 4: fallback to operator's own version
	return fmt.Sprintf("%s/%s:%s", defaultRegistry, componentName, fallbackTag)
}

func getComponentConfig(c *topology.ComponentsConfig, key string) *topology.ComponentImageConfig {
	switch key {
	case "devicePlugin":
		return c.DevicePlugin
	case "statusExporter":
		return c.StatusExporter
	case "kwokDraPlugin":
		return c.KwokDraPlugin
	default:
		return nil
	}
}
