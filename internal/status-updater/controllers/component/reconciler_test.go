package component

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kfake "k8s.io/client-go/kubernetes/fake"
)

var _ = Describe("Reconciler", func() {
	var (
		reconciler *Reconciler
		namespace  string
	)

	BeforeEach(func() {
		namespace = "fake-gpu-operator"
	})

	createTopologyCM := func(config *topology.ClusterConfig) *corev1.ConfigMap {
		data, err := yaml.Marshal(config)
		Expect(err).ToNot(HaveOccurred())
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "topology",
				Namespace: namespace,
			},
			Data: map[string]string{
				"topology.yml": string(data),
			},
		}
	}

	It("should create deployments for a new fake pool", func() {
		config := &topology.ClusterConfig{
			NodePoolLabelKey: "run.ai/simulated-gpu-node-pool",
			NodePools: map[string]topology.NodePoolConfig{
				"default": {Gpu: topology.GpuConfig{Backend: "fake"}},
			},
		}
		cm := createTopologyCM(config)
		client := kfake.NewSimpleClientset(cm)
		reconciler = NewReconciler(client, ReconcileParams{
			Namespace:       namespace,
			DefaultRegistry: "ghcr.io/run-ai/fake-gpu-operator",
			FallbackTag:     "0.5.0",
			PrometheusURL:   "http://prometheus:9090",
			DraEnabled:      false,
		})

		err := reconciler.Reconcile(context.Background())
		Expect(err).ToNot(HaveOccurred())

		deps, err := client.AppsV1().Deployments(namespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: constants.LabelManagedBy + "=" + constants.LabelManagedByValue,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(deps.Items).To(HaveLen(2)) // device-plugin + status-exporter

		names := make([]string, len(deps.Items))
		for i, d := range deps.Items {
			names[i] = d.Name
		}
		Expect(names).To(ContainElement("kwok-gpu-device-plugin-default"))
		Expect(names).To(ContainElement("kwok-status-exporter-default"))
	})

	It("should create services for a new fake pool", func() {
		config := &topology.ClusterConfig{
			NodePoolLabelKey: "run.ai/simulated-gpu-node-pool",
			NodePools: map[string]topology.NodePoolConfig{
				"default": {Gpu: topology.GpuConfig{Backend: "fake"}},
			},
		}
		cm := createTopologyCM(config)
		client := kfake.NewSimpleClientset(cm)
		reconciler = NewReconciler(client, ReconcileParams{
			Namespace:       namespace,
			DefaultRegistry: "ghcr.io/run-ai/fake-gpu-operator",
			FallbackTag:     "0.5.0",
			PrometheusURL:   "http://prometheus:9090",
		})

		err := reconciler.Reconcile(context.Background())
		Expect(err).ToNot(HaveOccurred())

		svcs, err := client.CoreV1().Services(namespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: constants.LabelManagedBy + "=" + constants.LabelManagedByValue,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(svcs.Items).To(HaveLen(1))
		Expect(svcs.Items[0].Name).To(Equal("kwok-status-exporter-default"))
	})

	It("should delete deployments when pool is removed", func() {
		config := &topology.ClusterConfig{
			NodePoolLabelKey: "run.ai/simulated-gpu-node-pool",
			NodePools:        map[string]topology.NodePoolConfig{},
		}
		cm := createTopologyCM(config)

		existingDep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kwok-gpu-device-plugin-default",
				Namespace: namespace,
				Labels: map[string]string{
					constants.LabelManagedBy: constants.LabelManagedByValue,
					constants.LabelComponent: constants.ComponentKwokDevicePlugin,
					constants.LabelPool:      "default",
				},
			},
		}
		client := kfake.NewSimpleClientset(cm, existingDep)
		reconciler = NewReconciler(client, ReconcileParams{
			Namespace:       namespace,
			DefaultRegistry: "ghcr.io/run-ai/fake-gpu-operator",
			FallbackTag:     "0.5.0",
			PrometheusURL:   "http://prometheus:9090",
		})

		err := reconciler.Reconcile(context.Background())
		Expect(err).ToNot(HaveOccurred())

		deps, err := client.AppsV1().Deployments(namespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: constants.LabelManagedBy + "=" + constants.LabelManagedByValue,
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(deps.Items).To(BeEmpty())
	})

	It("should update deployment when image changes", func() {
		config := &topology.ClusterConfig{
			NodePoolLabelKey: "run.ai/simulated-gpu-node-pool",
			NodePools: map[string]topology.NodePoolConfig{
				"default": {Gpu: topology.GpuConfig{Backend: "fake"}},
			},
			Components: &topology.ComponentsConfig{
				DevicePlugin: &topology.ComponentImageConfig{Image: "custom/dp:2.0.0"},
			},
		}
		cm := createTopologyCM(config)

		existingDep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kwok-gpu-device-plugin-default",
				Namespace: namespace,
				Labels: map[string]string{
					constants.LabelManagedBy: constants.LabelManagedByValue,
					constants.LabelComponent: constants.ComponentKwokDevicePlugin,
					constants.LabelPool:      "default",
					"app":                    "kwok-gpu-device-plugin-default",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Image: "custom/dp:1.0.0"}},
					},
				},
			},
		}
		client := kfake.NewSimpleClientset(cm, existingDep)
		reconciler = NewReconciler(client, ReconcileParams{
			Namespace:       namespace,
			DefaultRegistry: "ghcr.io/run-ai/fake-gpu-operator",
			FallbackTag:     "0.5.0",
			PrometheusURL:   "http://prometheus:9090",
		})

		err := reconciler.Reconcile(context.Background())
		Expect(err).ToNot(HaveOccurred())

		dep, err := client.AppsV1().Deployments(namespace).Get(context.Background(), "kwok-gpu-device-plugin-default", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(dep.Spec.Template.Spec.Containers[0].Image).To(Equal("custom/dp:2.0.0"))
	})
})
