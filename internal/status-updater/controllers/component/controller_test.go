package component

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kfake "k8s.io/client-go/kubernetes/fake"
)

var _ = Describe("ComponentController", func() {
	It("should reconcile on startup", func() {
		config := &topology.ClusterConfig{
			NodePoolLabelKey: "run.ai/simulated-gpu-node-pool",
			NodePools: map[string]topology.NodePoolConfig{
				"default": {Gpu: topology.GpuConfig{Backend: "fake"}},
			},
		}
		data, err := yaml.Marshal(config)
		Expect(err).ToNot(HaveOccurred())

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "topology",
				Namespace: "fake-gpu-operator",
			},
			Data: map[string]string{
				"topology.yml": string(data),
			},
		}
		client := kfake.NewSimpleClientset(cm)

		controller := NewComponentController(client, ReconcileParams{
			Namespace:       "fake-gpu-operator",
			DefaultRegistry: "ghcr.io/run-ai/fake-gpu-operator",
			FallbackTag:     "0.5.0",
			PrometheusURL:   "http://prometheus:9090",
		})

		stopCh := make(chan struct{})
		go controller.Run(stopCh)

		Eventually(func() int {
			deps, _ := client.AppsV1().Deployments("fake-gpu-operator").List(context.Background(), metav1.ListOptions{
				LabelSelector: constants.LabelManagedBy + "=" + constants.LabelManagedByValue,
			})
			return len(deps.Items)
		}, 5*time.Second, 100*time.Millisecond).Should(Equal(2))

		close(stopCh)
	})
})
