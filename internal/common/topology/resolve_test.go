package topology

import (
	"testing"

	"github.com/run-ai/fake-gpu-operator/internal/common/profile"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestResolveNodePool_WithProfile(t *testing.T) {
	profileData := `
system:
  driver_version: "550.163.01"
  cuda_version: "12.4"
device_defaults:
  name: "NVIDIA H100 80GB HBM3"
  architecture: "hopper"
  memory:
    total_bytes: 85899345920
devices:
  - index: 0
  - index: 1
  - index: 2
  - index: 3
  - index: 4
  - index: 5
  - index: 6
  - index: 7
`

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gpu-profile-h100",
			Namespace: "default",
		},
		Data: map[string]string{
			profile.CmProfileKey: profileData,
		},
	}

	client := fake.NewSimpleClientset(cm)

	pool := NodePoolConfig{
		Gpu: GpuConfig{
			Backend: "nvml-mock",
			Profile: "h100",
		},
	}

	resolved, err := ResolveNodePool(client, "default", pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.GpuProduct != "NVIDIA H100 80GB HBM3" {
		t.Errorf("GpuProduct = %q, want %q", resolved.GpuProduct, "NVIDIA H100 80GB HBM3")
	}
	if resolved.GpuMemory != 81920 {
		t.Errorf("GpuMemory = %d, want %d", resolved.GpuMemory, 81920)
	}
	if resolved.GpuCount != 8 {
		t.Errorf("GpuCount = %d, want %d", resolved.GpuCount, 8)
	}
}

func TestResolveNodePool_WithProfileAndOverrides(t *testing.T) {
	profileData := `
device_defaults:
  name: "NVIDIA H100 80GB HBM3"
  memory:
    total_bytes: 85899345920
devices:
  - index: 0
  - index: 1
`

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "gpu-profile-h100",
			Namespace: "default",
		},
		Data: map[string]string{
			profile.CmProfileKey: profileData,
		},
	}

	client := fake.NewSimpleClientset(cm)

	pool := NodePoolConfig{
		Gpu: GpuConfig{
			Profile: "h100",
			Overrides: map[string]interface{}{
				"device_defaults": map[string]interface{}{
					"name": "Custom H100",
				},
			},
		},
	}

	resolved, err := ResolveNodePool(client, "default", pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.GpuProduct != "Custom H100" {
		t.Errorf("GpuProduct = %q, want %q", resolved.GpuProduct, "Custom H100")
	}
	// Memory should come from the base profile
	if resolved.GpuMemory != 81920 {
		t.Errorf("GpuMemory = %d, want %d", resolved.GpuMemory, 81920)
	}
}

func TestResolveNodePool_FromOverrides_BackwardsCompat(t *testing.T) {
	// Simulates what normalizeNodePool produces from old format:
	// gpuProduct: "Tesla-K80", gpuMemory: 11441, gpuCount: 2
	client := fake.NewSimpleClientset()

	pool := NodePoolConfig{
		Gpu: GpuConfig{
			Backend: "fake",
			Overrides: map[string]interface{}{
				"device_defaults": map[string]interface{}{
					"name": "Tesla-K80",
					"memory": map[string]interface{}{
						"total_bytes": int64(11441) * 1024 * 1024,
					},
				},
				"devices": []interface{}{
					map[string]interface{}{},
					map[string]interface{}{},
				},
			},
		},
	}

	resolved, err := ResolveNodePool(client, "default", pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.GpuProduct != "Tesla-K80" {
		t.Errorf("GpuProduct = %q, want %q", resolved.GpuProduct, "Tesla-K80")
	}
	if resolved.GpuMemory != 11441 {
		t.Errorf("GpuMemory = %d, want %d", resolved.GpuMemory, 11441)
	}
	if resolved.GpuCount != 2 {
		t.Errorf("GpuCount = %d, want %d", resolved.GpuCount, 2)
	}
}

func TestResolveNodePool_Resources(t *testing.T) {
	client := fake.NewSimpleClientset()

	pool := NodePoolConfig{
		Gpu: GpuConfig{
			Backend: "fake",
			Overrides: map[string]interface{}{
				"device_defaults": map[string]interface{}{
					"name": "Tesla-T4",
				},
			},
		},
		Resources: []map[string]int{
			{"nvidia.com/gpu": 4},
			{"rdma/hca": 2},
		},
	}

	resolved, err := ResolveNodePool(client, "default", pool)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// nvidia.com/gpu should be excluded from OtherDevices
	if len(resolved.OtherDevices) != 1 {
		t.Fatalf("OtherDevices length = %d, want 1", len(resolved.OtherDevices))
	}
	if resolved.OtherDevices[0].Name != "rdma/hca" || resolved.OtherDevices[0].Count != 2 {
		t.Errorf("OtherDevices[0] = %+v, want {rdma/hca 2}", resolved.OtherDevices[0])
	}
}

func TestResolveNodePool_ProfileNotFound(t *testing.T) {
	client := fake.NewSimpleClientset()

	pool := NodePoolConfig{
		Gpu: GpuConfig{
			Profile: "nonexistent",
		},
	}

	_, err := ResolveNodePool(client, "default", pool)
	if err == nil {
		t.Fatal("expected error for missing profile, got nil")
	}
}
