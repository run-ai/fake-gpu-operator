package pod

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func gpuContainer(name string, gpus string) corev1.Container {
	container := corev1.Container{Name: name}
	if gpus != "" {
		container.Resources.Limits = corev1.ResourceList{
			constants.GpuResourceName: resource.MustParse(gpus),
		}
	}
	return container
}

func dedicatedGpuPod(containers ...corev1.Container) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dedicated-pod",
			Namespace: testNamespace,
			UID:       testPodUID,
		},
		Spec: corev1.PodSpec{
			NodeName:   testNodeName,
			Containers: containers,
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
}

func allocatedContainers(nodeTopology *topology.NodeTopology) []string {
	var containers []string
	for _, gpu := range nodeTopology.Gpus {
		containers = append(containers, gpu.Status.AllocatedBy.Container)
	}
	return containers
}

var _ = Describe("Dedicated GPU Pod Handler", func() {
	var (
		handler      *PodHandler
		nodeTopology *topology.NodeTopology
	)

	BeforeEach(func() {
		handler = NewPodHandler(fake.NewSimpleClientset(), nil)
		nodeTopology = &topology.NodeTopology{
			GpuMemory:  testGpuMemory,
			GpuProduct: testGpuProduct,
			Gpus: []topology.GpuDetails{
				{ID: testGpuID0, Status: topology.GpuStatus{PodGpuUsageStatus: make(topology.PodGpuUsageStatusMap)}},
				{ID: testGpuID1, Status: topology.GpuStatus{PodGpuUsageStatus: make(topology.PodGpuUsageStatusMap)}},
				{ID: testGpuID2, Status: topology.GpuStatus{PodGpuUsageStatus: make(topology.PodGpuUsageStatusMap)}},
			},
		}
	})

	Describe("IsDedicatedGpuPod", func() {
		It("is true when only a later container requests GPUs", func() {
			pod := dedicatedGpuPod(gpuContainer("sidecar", ""), gpuContainer("worker", "1"))
			Expect(util.IsDedicatedGpuPod(pod)).To(BeTrue())
		})

		It("is false when no container requests GPUs", func() {
			pod := dedicatedGpuPod(gpuContainer("sidecar", ""), gpuContainer("worker", "0"))
			Expect(util.IsDedicatedGpuPod(pod)).To(BeFalse())
		})
	})

	It("allocates GPUs requested by a container that is not first", func() {
		pod := dedicatedGpuPod(gpuContainer("sidecar", ""), gpuContainer("worker", "2"))

		Expect(handler.handleDedicatedGpuPodAddition(pod, nodeTopology)).To(Succeed())

		Expect(allocatedContainers(nodeTopology)).To(Equal([]string{"worker", "worker", ""}))
		Expect(nodeTopology.Gpus[0].Status.PodGpuUsageStatus).To(HaveKey(testPodUID))
		Expect(nodeTopology.Gpus[1].Status.PodGpuUsageStatus).To(HaveKey(testPodUID))
	})

	It("allocates GPUs for every container that requests them", func() {
		pod := dedicatedGpuPod(gpuContainer("a", "1"), gpuContainer("b", "2"))

		Expect(handler.handleDedicatedGpuPodAddition(pod, nodeTopology)).To(Succeed())

		Expect(allocatedContainers(nodeTopology)).To(Equal([]string{"a", "b", "b"}))
	})

	It("does not allocate twice when the pod is added again", func() {
		pod := dedicatedGpuPod(gpuContainer("sidecar", ""), gpuContainer("worker", "1"))

		Expect(handler.handleDedicatedGpuPodAddition(pod, nodeTopology)).To(Succeed())
		Expect(handler.handleDedicatedGpuPodAddition(pod, nodeTopology)).To(Succeed())

		Expect(allocatedContainers(nodeTopology)).To(Equal([]string{"worker", "", ""}))
	})

	It("updates usage on all of the pod's GPUs", func() {
		pod := dedicatedGpuPod(gpuContainer("a", "1"), gpuContainer("b", "1"))
		Expect(handler.handleDedicatedGpuPodAddition(pod, nodeTopology)).To(Succeed())
		for idx := range nodeTopology.Gpus {
			delete(nodeTopology.Gpus[idx].Status.PodGpuUsageStatus, testPodUID)
		}

		Expect(handler.handleDedicatedGpuPodUpdate(pod, nodeTopology)).To(Succeed())

		Expect(nodeTopology.Gpus[0].Status.PodGpuUsageStatus).To(HaveKey(testPodUID))
		Expect(nodeTopology.Gpus[1].Status.PodGpuUsageStatus).To(HaveKey(testPodUID))
	})

	It("releases every GPU the pod holds on deletion", func() {
		pod := dedicatedGpuPod(gpuContainer("a", "1"), gpuContainer("b", "2"))
		Expect(handler.handleDedicatedGpuPodAddition(pod, nodeTopology)).To(Succeed())

		handler.handleDedicatedGpuPodDeletion(pod, nodeTopology)

		Expect(allocatedContainers(nodeTopology)).To(Equal([]string{"", "", ""}))
	})
})
