package topology

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestComponentsConfigParsing(t *testing.T) {
	input := `
nodePools:
  default:
    gpu:
      backend: fake
      profile: A100-SXM4-80GB
components:
  imageTag: "0.5.0"
  imageRegistry: "gcr.io/run-ai-lab/fake-gpu-operator"
  devicePlugin:
    image: "gcr.io/custom/device-plugin:0.6.0"
  gpuOperator:
    chartVersion: "24.9.0"
nodePoolLabelKey: run.ai/simulated-gpu-node-pool
`
	config, err := ParseAndNormalizeTopology([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Components == nil {
		t.Fatal("expected Components to be non-nil")
	}
	if config.Components.ImageTag != "0.5.0" {
		t.Errorf("expected ImageTag 0.5.0, got %s", config.Components.ImageTag)
	}
	if config.Components.ImageRegistry != "gcr.io/run-ai-lab/fake-gpu-operator" {
		t.Errorf("expected ImageRegistry gcr.io/run-ai-lab/fake-gpu-operator, got %s", config.Components.ImageRegistry)
	}
	if config.Components.DevicePlugin == nil {
		t.Fatal("expected DevicePlugin to be non-nil")
	}
	if config.Components.DevicePlugin.Image != "gcr.io/custom/device-plugin:0.6.0" {
		t.Errorf("expected DevicePlugin.Image gcr.io/custom/device-plugin:0.6.0, got %s", config.Components.DevicePlugin.Image)
	}
	if config.Components.GpuOperator == nil {
		t.Fatal("expected GpuOperator to be non-nil")
	}
	if config.Components.GpuOperator.ChartVersion != "24.9.0" {
		t.Errorf("expected GpuOperator.ChartVersion 24.9.0, got %s", config.Components.GpuOperator.ChartVersion)
	}
}

func TestComponentsConfigMissing(t *testing.T) {
	input := `
nodePools:
  default:
    gpu:
      backend: fake
      profile: A100-SXM4-80GB
nodePoolLabelKey: run.ai/simulated-gpu-node-pool
`
	config, err := ParseAndNormalizeTopology([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Components != nil {
		t.Errorf("expected Components to be nil, got %+v", config.Components)
	}
}

func TestComponentsConfigRoundTrip(t *testing.T) {
	original := &ComponentsConfig{
		ImageTag:       "1.0.0",
		ImageRegistry:  "gcr.io/test",
		DevicePlugin:   &ComponentImageConfig{Image: "custom:latest"},
		StatusExporter: &ComponentImageConfig{ImageTag: "2.0.0"},
	}
	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed ComponentsConfig
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if parsed.ImageTag != "1.0.0" {
		t.Errorf("expected ImageTag 1.0.0, got %s", parsed.ImageTag)
	}
	if parsed.DevicePlugin == nil || parsed.DevicePlugin.Image != "custom:latest" {
		t.Errorf("expected DevicePlugin.Image custom:latest, got %+v", parsed.DevicePlugin)
	}
	if parsed.StatusExporter == nil || parsed.StatusExporter.ImageTag != "2.0.0" {
		t.Errorf("expected StatusExporter.ImageTag 2.0.0, got %+v", parsed.StatusExporter)
	}
}
