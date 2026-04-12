package runai

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
