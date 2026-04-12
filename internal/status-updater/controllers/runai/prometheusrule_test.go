package runai

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	discoveryfake "k8s.io/client-go/discovery/fake"
	dfake "k8s.io/client-go/dynamic/fake"
	kfake "k8s.io/client-go/kubernetes/fake"
)

func TestRunaiController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RunaiController Suite")
}

var _ = Describe("buildKwokPrometheusRule", func() {
	It("should build an unstructured PrometheusRule with the correct metadata", func() {
		rule := buildKwokPrometheusRule("runai")

		Expect(rule.GetName()).To(Equal("fake-gpu-operator-kwok-dcgm"))
		Expect(rule.GetNamespace()).To(Equal("runai"))
		Expect(rule.GetKind()).To(Equal("PrometheusRule"))
		Expect(rule.GetAPIVersion()).To(Equal("monitoring.coreos.com/v1"))

		labels := rule.GetLabels()
		Expect(labels["app"]).To(Equal("nvidia-dcgm-exporter"))
		Expect(labels["component"]).To(Equal("status-exporter-kwok"))
	})

	It("should contain the three KWOK DCGM recording rules", func() {
		rule := buildKwokPrometheusRule("runai")

		groups, found, err := unstructuredNestedSlice(rule.Object, "spec", "groups")
		Expect(err).ToNot(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(groups).To(HaveLen(1))

		group := groups[0].(map[string]interface{})
		Expect(group["name"]).To(Equal("kwok-dcgm-metrics"))

		rules := group["rules"].([]interface{})
		Expect(rules).To(HaveLen(3))

		recordNames := []string{}
		for _, r := range rules {
			ruleMap := r.(map[string]interface{})
			recordNames = append(recordNames, ruleMap["record"].(string))
		}
		Expect(recordNames).To(ConsistOf(
			"runai_dcgm_gpu_utilization",
			"runai_dcgm_gpu_used_mebibytes",
			"runai_dcgm_gpu_total_mebibytes",
		))
	})
})

var _ = Describe("RunaiController reconcile", func() {
	var (
		kubeClient    *kfake.Clientset
		dynamicClient *dfake.FakeDynamicClient
		discClient    *discoveryfake.FakeDiscovery
	)

	BeforeEach(func() {
		kubeClient = kfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		dynamicClient = dfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				prometheusRuleGVR: "PrometheusRuleList",
			},
		)
		discClient = kubeClient.Discovery().(*discoveryfake.FakeDiscovery)
		discClient.Resources = []*metav1.APIResourceList{
			{
				GroupVersion: "monitoring.coreos.com/v1",
				APIResources: []metav1.APIResource{
					{Name: "prometheusrules", Kind: "PrometheusRule"},
				},
			},
		}
	})

	When("runai namespace exists and PrometheusRule is missing", func() {
		BeforeEach(func() {
			ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "runai"}}
			_, err := kubeClient.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		It("should create the PrometheusRule", func() {
			ctrl := NewRunaiController(kubeClient, dynamicClient, 30*time.Second)
			err := ctrl.reconcile(context.Background())
			Expect(err).ToNot(HaveOccurred())

			rule, err := dynamicClient.Resource(prometheusRuleGVR).Namespace("runai").Get(
				context.Background(), prometheusRuleName, metav1.GetOptions{})
			Expect(err).ToNot(HaveOccurred())
			Expect(rule.GetName()).To(Equal(prometheusRuleName))
		})
	})

	When("runai namespace exists and PrometheusRule already exists", func() {
		BeforeEach(func() {
			ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "runai"}}
			_, err := kubeClient.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			rule := buildKwokPrometheusRule("runai")
			_, err = dynamicClient.Resource(prometheusRuleGVR).Namespace("runai").Create(
				context.Background(), rule, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		It("should not error (no-op)", func() {
			ctrl := NewRunaiController(kubeClient, dynamicClient, 30*time.Second)
			err := ctrl.reconcile(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})
	})

	When("runai namespace does not exist", func() {
		It("should not create the PrometheusRule", func() {
			ctrl := NewRunaiController(kubeClient, dynamicClient, 30*time.Second)
			err := ctrl.reconcile(context.Background())
			Expect(err).ToNot(HaveOccurred())

			_, err = dynamicClient.Resource(prometheusRuleGVR).Namespace("runai").Get(
				context.Background(), prometheusRuleName, metav1.GetOptions{})
			Expect(err).To(HaveOccurred())
		})
	})

	When("monitoring.coreos.com API is not available", func() {
		BeforeEach(func() {
			discClient.Resources = []*metav1.APIResourceList{}

			ns := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "runai"}}
			_, err := kubeClient.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		})

		It("should skip without error", func() {
			ctrl := NewRunaiController(kubeClient, dynamicClient, 30*time.Second)
			err := ctrl.reconcile(context.Background())
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
