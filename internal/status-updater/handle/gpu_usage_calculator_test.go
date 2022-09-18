package handle

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

var _ = Describe("GpuUsageCalculator", func() {
	When("GPU utilization is specified in an annotation", func() {
		const (
			totalGpuMemory = 1000
		)

		cases := map[string]struct {
			gpuUtilizationAnnotation string
			gpuFractionAnnotation    string
			expected                 topology.GpuUsageStatus
		}{
			"static GPU utilization": {
				gpuUtilizationAnnotation: "15",
				gpuFractionAnnotation:    "0.5",
				expected: topology.GpuUsageStatus{
					Utilization: topology.Range{
						Min: 15,
						Max: 15,
					},
					FbUsed:         500,
					IsInferencePod: false,
				},
			},
			"range GPU utilization": {
				gpuUtilizationAnnotation: "15-30",
				gpuFractionAnnotation:    "1",
				expected: topology.GpuUsageStatus{
					Utilization: topology.Range{
						Min: 15,
						Max: 30,
					},
					FbUsed:         1000,
					IsInferencePod: false,
				},
			},
		}

		for cName, cInfo := range cases {
			It(fmt.Sprintf("should calculate GpuUsageStatus correctly in case of a %s", cName), func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							gpuFractionAnnotationKey:    cInfo.gpuFractionAnnotation,
							gpuUtilizationAnnotationKey: cInfo.gpuUtilizationAnnotation,
						},
					},
				}

				actual := calculateUsage(nil, pod, totalGpuMemory)

				Expect(actual).To(Equal(cInfo.expected))
			})
		}
	})
})
