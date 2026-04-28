package mock

import (
	"context"
	"sync"
	"testing"
	"time"

	commonprofile "github.com/run-ai/fake-gpu-operator/internal/common/profile"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestController_ImplementsInterface(t *testing.T) {
	var _ controllers.Interface = (*MockController)(nil)
}

func TestController_ReconcilesOnTopologyChange(t *testing.T) {
	cfg := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools: map[string]topology.NodePoolConfig{
			"train": {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
		},
	}
	body, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	kube := fake.NewSimpleClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "topology", Namespace: "ns"},
			Data:       map[string]string{"topology.yml": string(body)},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "gpu-profile-a100", Namespace: "ns",
				Labels: map[string]string{commonprofile.LabelGpuProfile: "true"},
			},
			Data: map[string]string{commonprofile.CmProfileKey: `system: { driver_version: "550" }`},
		},
	)

	c := NewMockController(kube, ReconcileParams{
		Namespace: "ns", Image: "img:t",
	})

	stopCh := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.Run(stopCh)
	}()

	// Wait for the informer to fire AddFunc → Reconcile → DaemonSet appears.
	require.Eventually(t, func() bool {
		_, err := kube.AppsV1().DaemonSets("ns").
			Get(context.Background(), "nvml-mock-train", metav1.GetOptions{})
		return err == nil
	}, 3*time.Second, 50*time.Millisecond)

	close(stopCh)
	wg.Wait()

	ds, err := kube.AppsV1().DaemonSets("ns").
		Get(context.Background(), "nvml-mock-train", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "nvml-mock-train", ds.Name)
}
