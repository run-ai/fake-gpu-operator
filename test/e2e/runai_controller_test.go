package e2e_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var prometheusRuleGVR = schema.GroupVersionResource{
	Group:    "monitoring.coreos.com",
	Version:  "v1",
	Resource: "prometheusrules",
}

var _ = Describe("RunAI Controller Tests", func() {
	const (
		runaiNamespace     = "runai"
		prometheusRuleName = "fake-gpu-operator-kwok-dcgm"
	)

	AfterEach(func() {
		// Cleanup: delete runai namespace (cascades the PrometheusRule)
		_ = kubeClient.CoreV1().Namespaces().Delete(context.Background(), runaiNamespace, metav1.DeleteOptions{})
		// Wait for namespace to be fully deleted
		Eventually(func() bool {
			_, err := kubeClient.CoreV1().Namespaces().Get(context.Background(), runaiNamespace, metav1.GetOptions{})
			return err != nil
		}).WithTimeout(namespaceTimeout).Should(BeTrue(), "runai namespace should be deleted")
	})

	It("should not create PrometheusRule when runai namespace does not exist", func() {
		// Verify runai namespace doesn't exist
		_, err := kubeClient.CoreV1().Namespaces().Get(context.Background(), runaiNamespace, metav1.GetOptions{})
		Expect(err).To(HaveOccurred(), "runai namespace should not exist")

		// Verify PrometheusRule doesn't exist
		_, err = dynamicClient.Resource(prometheusRuleGVR).Namespace(runaiNamespace).Get(
			context.Background(), prometheusRuleName, metav1.GetOptions{})
		Expect(err).To(HaveOccurred(), "PrometheusRule should not exist without runai namespace")
	})

	It("should create PrometheusRule when runai namespace is created", func() {
		// Step 1: Verify no PrometheusRule exists initially
		_, err := dynamicClient.Resource(prometheusRuleGVR).Namespace(runaiNamespace).Get(
			context.Background(), prometheusRuleName, metav1.GetOptions{})
		Expect(err).To(HaveOccurred(), "PrometheusRule should not exist initially")

		// Step 2: Create runai namespace
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: runaiNamespace}}
		_, err = kubeClient.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred(), "Should create runai namespace")

		// Step 3: Wait for the status-updater's runai controller to create the PrometheusRule
		// The controller polls every 5s (set in test values.yaml)
		Eventually(func() error {
			rule, err := dynamicClient.Resource(prometheusRuleGVR).Namespace(runaiNamespace).Get(
				context.Background(), prometheusRuleName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			// Verify labels
			labels := rule.GetLabels()
			Expect(labels["app"]).To(Equal("nvidia-dcgm-exporter"))
			Expect(labels["component"]).To(Equal("status-exporter-kwok"))

			return nil
		}).WithTimeout(testTimeout).WithPolling(2 * time.Second).Should(Succeed(),
			"PrometheusRule should be created by runai controller after namespace appears")
	})
})
