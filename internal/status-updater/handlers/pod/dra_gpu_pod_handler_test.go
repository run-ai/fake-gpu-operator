package pod

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/util"
	corev1 "k8s.io/api/core/v1"
	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	testNamespace     = "default"
	testNodeName      = "node1"
	testPodName       = "dra-pod"
	testPodUID        = types.UID("pod-uid-123")
	testClaimName     = "my-gpu-claim"
	testContainerName = "main"
	testGpuMemory     = 16000
	testGpuProduct    = "NVIDIA-A100"

	testGpuID0 = "gpu-0001-0001-0001-0001"
	testGpuID1 = "gpu-0002-0002-0002-0002"
	testGpuID2 = "gpu-0003-0003-0003-0003"
)

var _ = Describe("DRA GPU Pod Handler", func() {
	var (
		handler      *PodHandler
		fakeClient   *fake.Clientset
		nodeTopology *topology.NodeTopology
	)

	BeforeEach(func() {
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

	Describe("IsDraPod", func() {
		It("should return true for pods with ResourceClaims", func() {
			claimName := testClaimName
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testPodName,
					Namespace: testNamespace,
				},
				Spec: corev1.PodSpec{
					ResourceClaims: []corev1.PodResourceClaim{
						{Name: "gpu", ResourceClaimName: &claimName},
					},
				},
			}
			Expect(util.IsDraPod(pod)).To(BeTrue())
		})

		It("should return false for pods without ResourceClaims", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "legacy-pod",
					Namespace: testNamespace,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: testContainerName},
					},
				},
			}
			Expect(util.IsDraPod(pod)).To(BeFalse())
		})
	})

	Describe("getResourceClaimNamesFromPod", func() {
		It("should extract claim names from pod.Status.ResourceClaimStatuses (template-based claims)", func() {
			claimName := "generated-claim-abc123"
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testPodName,
					Namespace: testNamespace,
				},
				Spec: corev1.PodSpec{
					ResourceClaims: []corev1.PodResourceClaim{
						{Name: "gpu", ResourceClaimTemplateName: ptrString("gpu-template")},
					},
				},
				Status: corev1.PodStatus{
					ResourceClaimStatuses: []corev1.PodResourceClaimStatus{
						{Name: "gpu", ResourceClaimName: &claimName},
					},
				},
			}

			claimNames := getResourceClaimNamesFromPod(pod)
			Expect(claimNames).To(HaveLen(1))
			Expect(claimNames[0]).To(Equal("generated-claim-abc123"))
		})

		It("should extract claim names from pod.Spec.ResourceClaims (direct references)", func() {
			claimName := testClaimName
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testPodName,
					Namespace: testNamespace,
				},
				Spec: corev1.PodSpec{
					ResourceClaims: []corev1.PodResourceClaim{
						{Name: "gpu", ResourceClaimName: &claimName},
					},
				},
			}

			claimNames := getResourceClaimNamesFromPod(pod)
			Expect(claimNames).To(HaveLen(1))
			Expect(claimNames[0]).To(Equal(testClaimName))
		})

		It("should deduplicate claim names", func() {
			claimName := "same-claim"
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testPodName,
					Namespace: testNamespace,
				},
				Spec: corev1.PodSpec{
					ResourceClaims: []corev1.PodResourceClaim{
						{Name: "gpu", ResourceClaimName: &claimName},
					},
				},
				Status: corev1.PodStatus{
					ResourceClaimStatuses: []corev1.PodResourceClaimStatus{
						{Name: "gpu", ResourceClaimName: &claimName},
					},
				},
			}

			claimNames := getResourceClaimNamesFromPod(pod)
			Expect(claimNames).To(HaveLen(1))
			Expect(claimNames[0]).To(Equal("same-claim"))
		})
	})

	Describe("getDevicesFromClaim", func() {
		It("should extract device names from allocated claim", func() {
			claim := &resourceapi.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testClaimName,
					Namespace: testNamespace,
				},
				Status: resourceapi.ResourceClaimStatus{
					Allocation: &resourceapi.AllocationResult{
						Devices: resourceapi.DeviceAllocationResult{
							Results: []resourceapi.DeviceRequestAllocationResult{
								{Driver: draDriverName, Device: testGpuID0, Pool: testNodeName, Request: "gpu"},
								{Driver: draDriverName, Device: testGpuID1, Pool: testNodeName, Request: "gpu"},
							},
						},
					},
				},
			}

			devices := getDevicesFromClaim(claim)
			Expect(devices).To(HaveLen(2))
			Expect(devices).To(ContainElements(testGpuID0, testGpuID1))
		})

		It("should ignore devices from other drivers", func() {
			claim := &resourceapi.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-claim",
					Namespace: testNamespace,
				},
				Status: resourceapi.ResourceClaimStatus{
					Allocation: &resourceapi.AllocationResult{
						Devices: resourceapi.DeviceAllocationResult{
							Results: []resourceapi.DeviceRequestAllocationResult{
								{Driver: draDriverName, Device: testGpuID0, Pool: testNodeName, Request: "gpu"},
								{Driver: "other.driver.io", Device: "other-device", Pool: testNodeName, Request: "other"},
							},
						},
					},
				},
			}

			devices := getDevicesFromClaim(claim)
			Expect(devices).To(HaveLen(1))
			Expect(devices[0]).To(Equal(testGpuID0))
		})

		It("should return nil for unallocated claims", func() {
			claim := &resourceapi.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testClaimName,
					Namespace: testNamespace,
				},
				Status: resourceapi.ResourceClaimStatus{
					Allocation: nil,
				},
			}

			devices := getDevicesFromClaim(claim)
			Expect(devices).To(BeNil())
		})
	})

	Describe("handleDraGpuPodAddition", func() {
		BeforeEach(func() {
			// Create ResourceClaim with allocated GPUs
			claim := &resourceapi.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testClaimName,
					Namespace: testNamespace,
				},
				Status: resourceapi.ResourceClaimStatus{
					Allocation: &resourceapi.AllocationResult{
						Devices: resourceapi.DeviceAllocationResult{
							Results: []resourceapi.DeviceRequestAllocationResult{
								{Driver: draDriverName, Device: testGpuID0, Pool: testNodeName, Request: "gpu"},
							},
						},
					},
				},
			}

			fakeClient = fake.NewClientset(claim)
			handler = &PodHandler{
				kubeClient:    fakeClient,
				dynamicClient: nil,
			}
		})

		It("should allocate GPUs from ResourceClaim to node topology", func() {
			claimName := testClaimName
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testPodName,
					Namespace: testNamespace,
					UID:       testPodUID,
				},
				Spec: corev1.PodSpec{
					NodeName: testNodeName,
					Containers: []corev1.Container{
						{
							Name: testContainerName,
							Resources: corev1.ResourceRequirements{
								Claims: []corev1.ResourceClaim{{Name: "gpu"}},
							},
						},
					},
					ResourceClaims: []corev1.PodResourceClaim{
						{Name: "gpu", ResourceClaimName: &claimName},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			}

			err := handler.handleDraGpuPodAddition(pod, nodeTopology)
			Expect(err).NotTo(HaveOccurred())

			// Verify GPU is allocated
			Expect(nodeTopology.Gpus[0].Status.AllocatedBy.Pod).To(Equal(testPodName))
			Expect(nodeTopology.Gpus[0].Status.AllocatedBy.Namespace).To(Equal(testNamespace))
			Expect(nodeTopology.Gpus[0].Status.AllocatedBy.Container).To(Equal(testContainerName))

			// Other GPUs should remain free
			Expect(nodeTopology.Gpus[1].Status.AllocatedBy.Pod).To(BeEmpty())
			Expect(nodeTopology.Gpus[2].Status.AllocatedBy.Pod).To(BeEmpty())
		})

		It("should skip non-DRA pods", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "legacy-pod",
					Namespace: testNamespace,
				},
				Spec: corev1.PodSpec{
					NodeName: testNodeName,
					Containers: []corev1.Container{
						{Name: testContainerName},
					},
				},
			}

			err := handler.handleDraGpuPodAddition(pod, nodeTopology)
			Expect(err).NotTo(HaveOccurred())

			// All GPUs should remain free
			for _, gpu := range nodeTopology.Gpus {
				Expect(gpu.Status.AllocatedBy.Pod).To(BeEmpty())
			}
		})

		It("should skip already allocated pods", func() {
			// Pre-allocate GPU
			nodeTopology.Gpus[0].Status.AllocatedBy = topology.ContainerDetails{
				Namespace: testNamespace,
				Pod:       testPodName,
				Container: testContainerName,
			}

			claimName := testClaimName
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testPodName,
					Namespace: testNamespace,
				},
				Spec: corev1.PodSpec{
					NodeName: testNodeName,
					Containers: []corev1.Container{
						{Name: testContainerName},
					},
					ResourceClaims: []corev1.PodResourceClaim{
						{Name: "gpu", ResourceClaimName: &claimName},
					},
				},
			}

			err := handler.handleDraGpuPodAddition(pod, nodeTopology)
			Expect(err).NotTo(HaveOccurred())

			// GPU allocation should remain unchanged
			Expect(nodeTopology.Gpus[0].Status.AllocatedBy.Pod).To(Equal(testPodName))
		})
	})

	Describe("handleDraGpuPodDeletion", func() {
		BeforeEach(func() {
			fakeClient = fake.NewClientset()
			handler = &PodHandler{
				kubeClient:    fakeClient,
				dynamicClient: nil,
			}

			// Pre-allocate GPUs
			nodeTopology.Gpus[0].Status.AllocatedBy = topology.ContainerDetails{
				Namespace: testNamespace,
				Pod:       testPodName,
				Container: testContainerName,
			}
			nodeTopology.Gpus[0].Status.PodGpuUsageStatus[testPodUID] = topology.GpuUsageStatus{
				Utilization: topology.Range{Min: 50, Max: 100},
			}
		})

		It("should release GPU when DRA pod is deleted", func() {
			claimName := testClaimName
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testPodName,
					Namespace: testNamespace,
					UID:       testPodUID,
				},
				Spec: corev1.PodSpec{
					NodeName: testNodeName,
					Containers: []corev1.Container{
						{Name: testContainerName},
					},
					ResourceClaims: []corev1.PodResourceClaim{
						{Name: "gpu", ResourceClaimName: &claimName},
					},
				},
			}

			handler.handleDraGpuPodDeletion(pod, nodeTopology)

			// GPU should be released
			Expect(nodeTopology.Gpus[0].Status.AllocatedBy.Pod).To(BeEmpty())
			Expect(nodeTopology.Gpus[0].Status.AllocatedBy.Namespace).To(BeEmpty())
			Expect(nodeTopology.Gpus[0].Status.AllocatedBy.Container).To(BeEmpty())
		})

		It("should skip non-DRA pods", func() {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testPodName,
					Namespace: testNamespace,
				},
				Spec: corev1.PodSpec{
					NodeName: testNodeName,
					Containers: []corev1.Container{
						{Name: testContainerName},
					},
				},
			}

			handler.handleDraGpuPodDeletion(pod, nodeTopology)

			// GPU allocation should remain unchanged (pod is not DRA)
			Expect(nodeTopology.Gpus[0].Status.AllocatedBy.Pod).To(Equal(testPodName))
		})
	})

	Describe("findGpuIndexByID", func() {
		It("should find GPU by exact ID", func() {
			idx := findGpuIndexByID(nodeTopology, testGpuID1)
			Expect(idx).To(Equal(1))
		})

		It("should return -1 for non-existent GPU", func() {
			idx := findGpuIndexByID(nodeTopology, "gpu-9999-9999-9999-9999")
			Expect(idx).To(Equal(-1))
		})
	})

	Describe("getContainerWithClaim", func() {
		It("should return container name that has resource claims", func() {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "init-container"},
						{
							Name: "gpu-container",
							Resources: corev1.ResourceRequirements{
								Claims: []corev1.ResourceClaim{{Name: "gpu"}},
							},
						},
					},
				},
			}

			containerName := getContainerWithClaim(pod)
			Expect(containerName).To(Equal("gpu-container"))
		})

		It("should return first container as fallback", func() {
			pod := &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main"},
					},
				},
			}

			containerName := getContainerWithClaim(pod)
			Expect(containerName).To(Equal("main"))
		})
	})
})

// Helper function to create a string pointer
func ptrString(s string) *string {
	return &s
}

// Ensure runtime is imported for fake client
var _ = runtime.Object(&resourceapi.ResourceClaim{})
