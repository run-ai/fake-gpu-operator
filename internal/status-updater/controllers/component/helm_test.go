package component

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GpuOperatorHelmManager", func() {
	Describe("BuildGpuOperatorValues", func() {
		It("should aggregate node selectors from multiple mock pools", func() {
			pools := []string{"training", "inference"}
			nodePoolLabelKey := "run.ai/simulated-gpu-node-pool"

			values := BuildGpuOperatorValues(pools, nodePoolLabelKey)

			Expect(values["driver"]).To(Equal(map[string]interface{}{"enabled": false}))
			Expect(values["toolkit"]).To(Equal(map[string]interface{}{"enabled": false}))

			dp, ok := values["devicePlugin"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			affinity, ok := dp["affinity"].(map[string]interface{})
			Expect(ok).To(BeTrue())

			na := affinity["nodeAffinity"].(map[string]interface{})
			required := na["requiredDuringSchedulingIgnoredDuringExecution"].(map[string]interface{})
			terms := required["nodeSelectorTerms"].([]interface{})
			Expect(terms).To(HaveLen(1))
			term := terms[0].(map[string]interface{})
			exprs := term["matchExpressions"].([]interface{})
			Expect(exprs).To(HaveLen(1))
			expr := exprs[0].(map[string]interface{})
			Expect(expr["key"]).To(Equal(nodePoolLabelKey))
			Expect(expr["operator"]).To(Equal("In"))
			poolValues := expr["values"].([]interface{})
			Expect(poolValues).To(ConsistOf("inference", "training"))
		})

		It("should handle single mock pool", func() {
			pools := []string{"default"}
			values := BuildGpuOperatorValues(pools, "run.ai/simulated-gpu-node-pool")
			Expect(values).ToNot(BeNil())
			Expect(values["driver"]).To(Equal(map[string]interface{}{"enabled": false}))
		})

		It("should include managed-by label in daemonsets", func() {
			values := BuildGpuOperatorValues([]string{"pool1"}, "key")
			ds := values["daemonsets"].(map[string]interface{})
			labels := ds["labels"].(map[string]interface{})
			Expect(labels["app.kubernetes.io/managed-by"]).To(Equal("fake-gpu-operator"))
		})
	})
})
