package resourceslice

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"gopkg.in/yaml.v3"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKWOKDraPlugin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "KWOK DRA Plugin Suite")
}

var _ = Describe("ResourceSliceHandler", func() {
	Describe("HandleAddOrUpdate", func() {
		It("should create a ResourceSlice for a KWOK node", func() {
			nodeName := "kwok-node1"
			nodeTopology := &topology.NodeTopology{
				GpuProduct: "NVIDIA-A100-SXM4-40GB",
				GpuMemory:  40960,
				Gpus: []topology.GpuDetails{
					{ID: "GPU-0001-0001-0001-0001"},
					{ID: "GPU-0002-0002-0002-0002"},
				},
			}

			topologyData, err := yaml.Marshal(nodeTopology)
			Expect(err).NotTo(HaveOccurred())

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Labels: map[string]string{
						constants.LabelTopologyCMNodeName: nodeName,
					},
				},
				Data: map[string]string{
					topology.CmTopologyKey: string(topologyData),
				},
			}

			fakeClient := fake.NewSimpleClientset()

			handler := NewResourceSliceHandler(fakeClient, nil)
			err = handler.HandleAddOrUpdate(configMap)
			Expect(err).NotTo(HaveOccurred())

			// Verify ResourceSlice was created
			resourceSlice, err := fakeClient.ResourceV1().ResourceSlices().Get(context.TODO(), "kwok-kwok-node1-gpu", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resourceSlice).NotTo(BeNil())
			Expect(resourceSlice.Spec.Driver).To(Equal(DriverName))
			Expect(*resourceSlice.Spec.NodeName).To(Equal(nodeName))
			Expect(resourceSlice.Spec.Devices).To(HaveLen(2))
		})

		It("should update an existing ResourceSlice", func() {
			nodeName := "kwok-node2"
			nodeTopology := &topology.NodeTopology{
				GpuProduct: "NVIDIA-A100-SXM4-40GB",
				GpuMemory:  40960,
				Gpus: []topology.GpuDetails{
					{ID: "GPU-0001-0001-0001-0001"},
				},
			}

			topologyData, err := yaml.Marshal(nodeTopology)
			Expect(err).NotTo(HaveOccurred())

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Labels: map[string]string{
						constants.LabelTopologyCMNodeName: nodeName,
					},
				},
				Data: map[string]string{
					topology.CmTopologyKey: string(topologyData),
				},
			}

			fakeClient := fake.NewSimpleClientset()

			handler := NewResourceSliceHandler(fakeClient, nil)

			// Create initial ResourceSlice
			err = handler.HandleAddOrUpdate(configMap)
			Expect(err).NotTo(HaveOccurred())

			// Verify initial state
			resourceSlice, err := fakeClient.ResourceV1().ResourceSlices().Get(context.TODO(), "kwok-kwok-node2-gpu", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resourceSlice.Spec.Devices).To(HaveLen(1))

			// Update topology with more GPUs
			nodeTopology.Gpus = append(nodeTopology.Gpus, topology.GpuDetails{ID: "GPU-0002-0002-0002-0002"})
			topologyData, err = yaml.Marshal(nodeTopology)
			Expect(err).NotTo(HaveOccurred())
			configMap.Data[topology.CmTopologyKey] = string(topologyData)

			// Update ResourceSlice
			err = handler.HandleAddOrUpdate(configMap)
			Expect(err).NotTo(HaveOccurred())

			// Verify updated state
			resourceSlice, err = fakeClient.ResourceV1().ResourceSlices().Get(context.TODO(), "kwok-kwok-node2-gpu", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resourceSlice.Spec.Devices).To(HaveLen(2))
		})
	})

	Describe("HandleDelete", func() {
		It("should delete the ResourceSlice for a KWOK node", func() {
			nodeName := "kwok-node3"
			nodeTopology := &topology.NodeTopology{
				GpuProduct: "NVIDIA-A100-SXM4-40GB",
				GpuMemory:  40960,
				Gpus: []topology.GpuDetails{
					{ID: "GPU-0001-0001-0001-0001"},
				},
			}

			topologyData, err := yaml.Marshal(nodeTopology)
			Expect(err).NotTo(HaveOccurred())

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Labels: map[string]string{
						constants.LabelTopologyCMNodeName: nodeName,
					},
				},
				Data: map[string]string{
					topology.CmTopologyKey: string(topologyData),
				},
			}

			fakeClient := fake.NewSimpleClientset()

			handler := NewResourceSliceHandler(fakeClient, nil)

			// Create ResourceSlice
			err = handler.HandleAddOrUpdate(configMap)
			Expect(err).NotTo(HaveOccurred())

			// Verify it exists
			_, err = fakeClient.ResourceV1().ResourceSlices().Get(context.TODO(), "kwok-kwok-node3-gpu", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Delete ResourceSlice
			err = handler.HandleDelete(nodeName)
			Expect(err).NotTo(HaveOccurred())

			// Verify it's gone
			_, err = fakeClient.ResourceV1().ResourceSlices().Get(context.TODO(), "kwok-kwok-node3-gpu", metav1.GetOptions{})
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("devicesFromTopology", func() {
		It("should convert topology GPUs to DRA devices", func() {
			handler := &ResourceSliceHandler{}

			nodeTopology := &topology.NodeTopology{
				GpuProduct: "NVIDIA-A100-SXM4-40GB",
				GpuMemory:  40960,
				Gpus: []topology.GpuDetails{
					{ID: "GPU-0001-0001-0001-0001"},
					{ID: "GPU-0002-0002-0002-0002"},
				},
			}

			devices := handler.devicesFromTopology(nodeTopology)

			Expect(devices).To(HaveLen(2))

			// Check first device
			Expect(devices[0].Name).To(Equal("gpu-0001-0001-0001-0001"))
			Expect(*devices[0].Attributes["uuid"].StringValue).To(Equal("GPU-0001-0001-0001-0001"))
			Expect(*devices[0].Attributes["model"].StringValue).To(Equal("NVIDIA-A100-SXM4-40GB"))

			// Check second device
			Expect(devices[1].Name).To(Equal("gpu-0002-0002-0002-0002"))
			Expect(*devices[1].Attributes["uuid"].StringValue).To(Equal("GPU-0002-0002-0002-0002"))
		})

		It("should skip GPUs without ID", func() {
			handler := &ResourceSliceHandler{}

			nodeTopology := &topology.NodeTopology{
				GpuProduct: "NVIDIA-A100-SXM4-40GB",
				GpuMemory:  40960,
				Gpus: []topology.GpuDetails{
					{ID: "GPU-0001-0001-0001-0001"},
					{ID: ""}, // Empty ID, should be skipped
				},
			}

			devices := handler.devicesFromTopology(nodeTopology)

			Expect(devices).To(HaveLen(1))
			Expect(devices[0].Name).To(Equal("gpu-0001-0001-0001-0001"))
		})
	})
})
