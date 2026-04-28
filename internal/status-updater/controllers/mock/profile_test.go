package mock

import (
	"testing"

	commonprofile "github.com/run-ai/fake-gpu-operator/internal/common/profile"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/yaml"
)

const a100ProfileYAML = `
version: "1.0"
system:
  driver_version: "550.163.01"
  cuda_version: "12.4"
device_defaults:
  name: "NVIDIA A100-SXM4-40GB"
  architecture: "ampere"
devices:
  - index: 0
    uuid: "GPU-00000000-0000-0000-0000-000000000000"
`

func newProfileCM(name, ns, data string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    map[string]string{commonprofile.LabelGpuProfile: "true"},
		},
		Data: map[string]string{commonprofile.CmProfileKey: data},
	}
}

func TestRenderConfig_NoOverrides(t *testing.T) {
	cm := newProfileCM("gpu-profile-a100", "ns", a100ProfileYAML)
	kube := fake.NewSimpleClientset(cm)

	out, err := RenderConfig(kube, "ns", topology.GpuConfig{Profile: "a100"})
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, yaml.Unmarshal(out, &got))
	system := got["system"].(map[string]interface{})
	assert.Equal(t, "550.163.01", system["driver_version"])
}

func TestRenderConfig_ScalarOverride(t *testing.T) {
	cm := newProfileCM("gpu-profile-a100", "ns", a100ProfileYAML)
	kube := fake.NewSimpleClientset(cm)

	out, err := RenderConfig(kube, "ns", topology.GpuConfig{
		Profile: "a100",
		Overrides: map[string]interface{}{
			"system": map[string]interface{}{
				"driver_version": "999.99.99",
			},
		},
	})
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, yaml.Unmarshal(out, &got))
	system := got["system"].(map[string]interface{})
	assert.Equal(t, "999.99.99", system["driver_version"])
	assert.Equal(t, "12.4", system["cuda_version"], "non-overridden keys preserved")
}

func TestRenderConfig_NestedOverride(t *testing.T) {
	cm := newProfileCM("gpu-profile-a100", "ns", a100ProfileYAML)
	kube := fake.NewSimpleClientset(cm)

	out, err := RenderConfig(kube, "ns", topology.GpuConfig{
		Profile: "a100",
		Overrides: map[string]interface{}{
			"device_defaults": map[string]interface{}{
				"architecture": "custom-arch",
			},
		},
	})
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, yaml.Unmarshal(out, &got))
	dd := got["device_defaults"].(map[string]interface{})
	assert.Equal(t, "custom-arch", dd["architecture"])
	assert.Equal(t, "NVIDIA A100-SXM4-40GB", dd["name"], "non-overridden nested keys preserved")
}

func TestRenderConfig_AddNewKey(t *testing.T) {
	cm := newProfileCM("gpu-profile-a100", "ns", a100ProfileYAML)
	kube := fake.NewSimpleClientset(cm)

	out, err := RenderConfig(kube, "ns", topology.GpuConfig{
		Profile: "a100",
		Overrides: map[string]interface{}{
			"new_section": map[string]interface{}{
				"flag": true,
			},
		},
	})
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, yaml.Unmarshal(out, &got))
	newSec := got["new_section"].(map[string]interface{})
	assert.Equal(t, true, newSec["flag"])
}

func TestRenderConfig_MissingProfileCM(t *testing.T) {
	kube := fake.NewSimpleClientset()
	_, err := RenderConfig(kube, "ns", topology.GpuConfig{Profile: "h100"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "h100")
}

func TestRenderConfig_MalformedProfileYAML(t *testing.T) {
	cm := newProfileCM("gpu-profile-broken", "ns", "this: is: not: yaml: nested: bad")
	kube := fake.NewSimpleClientset(cm)
	_, err := RenderConfig(kube, "ns", topology.GpuConfig{Profile: "broken"})
	require.Error(t, err)
}

func TestRenderConfig_EmptyProfile(t *testing.T) {
	out, err := RenderConfig(fake.NewSimpleClientset(), "ns", topology.GpuConfig{})
	assert.Error(t, err, "empty profile name must error")
	assert.Nil(t, out)
}
