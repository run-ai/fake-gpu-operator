package node

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("generateGpuDetails", func() {
	const nodeName = "worker-1"

	It("sets NUMANode for indices present in the GpuNUMA map", func() {
		gpus := generateGpuDetails(4, nodeName, map[int]int{0: 0, 1: 0, 2: 1, 3: 1})
		Expect(gpus).To(HaveLen(4))
		for i, want := range []int{0, 0, 1, 1} {
			Expect(gpus[i].NUMANode).NotTo(BeNil())
			Expect(*gpus[i].NUMANode).To(Equal(want))
		}
	})

	It("leaves NUMANode nil when the GpuNUMA map is nil", func() {
		gpus := generateGpuDetails(2, nodeName, nil)
		Expect(gpus).To(HaveLen(2))
		Expect(gpus[0].NUMANode).To(BeNil())
		Expect(gpus[1].NUMANode).To(BeNil())
	})

	It("leaves NUMANode nil for indices absent from a non-nil GpuNUMA map", func() {
		gpus := generateGpuDetails(4, nodeName, map[int]int{0: 0, 2: 1}) // indices 1 and 3 missing
		Expect(gpus).To(HaveLen(4))
		Expect(gpus[0].NUMANode).NotTo(BeNil())
		Expect(*gpus[0].NUMANode).To(Equal(0))
		Expect(gpus[1].NUMANode).To(BeNil())
		Expect(gpus[2].NUMANode).NotTo(BeNil())
		Expect(*gpus[2].NUMANode).To(Equal(1))
		Expect(gpus[3].NUMANode).To(BeNil())
	})

	It("assigns a stable ID independent of NUMA", func() {
		withNUMA := generateGpuDetails(1, nodeName, map[int]int{0: 1})
		withoutNUMA := generateGpuDetails(1, nodeName, nil)
		Expect(withNUMA[0].ID).To(Equal(withoutNUMA[0].ID))
	})
})
