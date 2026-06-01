package deviceplugin

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"

	"k8s.io/client-go/kubernetes/fake"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

func TestDevicePlugin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DevicePlugin Suite")
}

var _ = Describe("NewDevicePlugins", func() {
	Context("When the topology is nil", func() {
		It("should panic", func() {
			Expect(func() { NewDevicePlugins(nil, nil) }).To(Panic())
		})
	})

	Context("When the fake node is enabled", Ordered, func() {
		BeforeAll(func() {
			viper.Set(constants.EnvFakeNode, true)
		})

		AfterAll(func() {
			viper.Set(constants.EnvFakeNode, false)
		})

		It("should return a fake node device plugin", func() {
			topology := &topology.NodeTopology{}
			kubeClient := &fake.Clientset{}
			devicePlugins := NewDevicePlugins(topology, kubeClient)
			Expect(devicePlugins).To(HaveLen(1))
			Expect(devicePlugins[0]).To(BeAssignableToTypeOf(&FakeNodeDevicePlugin{}))
		})
	})

	Context("With normal node", func() {
		It("should return a real node device plugin", func() {
			topology := &topology.NodeTopology{}
			kubeClient := &fake.Clientset{}
			devicePlugins := NewDevicePlugins(topology, kubeClient)
			Expect(devicePlugins).To(HaveLen(1))
			Expect(devicePlugins[0]).To(BeAssignableToTypeOf(&RealNodeDevicePlugin{}))
		})

		It("should return a device plugin for each other device", func() {
			topology := &topology.NodeTopology{
				OtherDevices: []topology.GenericDevice{
					{Name: "device1", Count: 1},
					{Name: "device2", Count: 2},
				},
			}
			kubeClient := &fake.Clientset{}
			devicePlugins := NewDevicePlugins(topology, kubeClient)
			Expect(devicePlugins).To(HaveLen(3))
			Expect(devicePlugins[0]).To(BeAssignableToTypeOf(&RealNodeDevicePlugin{}))
		})

		It("should return MIG device plugins for mixed MIG topology", func() {
			topology := &topology.NodeTopology{
				MigStrategy: "mixed",
				Gpus: []topology.GpuDetails{
					{
						ID:         "GPU-0",
						MigEnabled: true,
						MigInstances: []topology.MigInstance{
							{Profile: "1g.5gb"},
							{Profile: "1g.5gb"},
						},
					},
				},
			}
			kubeClient := &fake.Clientset{}
			devicePlugins := NewDevicePlugins(topology, kubeClient)
			Expect(devicePlugins).To(HaveLen(1))

			plugin, ok := devicePlugins[0].(*RealNodeDevicePlugin)
			Expect(ok).To(BeTrue())
			Expect(plugin.resourceName).To(Equal("nvidia.com/mig-1g.5gb"))
			Expect(plugin.devs).To(HaveLen(2))
		})

		It("should return one device plugin per MIG profile", func() {
			topology := &topology.NodeTopology{
				MigStrategy: "mixed",
				Gpus: []topology.GpuDetails{
					{
						ID:         "GPU-0",
						MigEnabled: true,
						MigInstances: []topology.MigInstance{
							{Profile: "1g.5gb"},
							{Profile: "2g.10gb"},
						},
					},
				},
			}
			kubeClient := &fake.Clientset{}
			devicePlugins := NewDevicePlugins(topology, kubeClient)
			Expect(devicePlugins).To(HaveLen(2))

			mig1g, ok := devicePlugins[0].(*RealNodeDevicePlugin)
			Expect(ok).To(BeTrue())
			Expect(mig1g.resourceName).To(Equal("nvidia.com/mig-1g.5gb"))

			mig2g, ok := devicePlugins[1].(*RealNodeDevicePlugin)
			Expect(ok).To(BeTrue())
			Expect(mig2g.resourceName).To(Equal("nvidia.com/mig-2g.10gb"))
		})

		It("should keep full GPUs alongside MIG profiles in mixed mode", func() {
			topology := &topology.NodeTopology{
				MigStrategy: "mixed",
				Gpus: []topology.GpuDetails{
					{
						ID:         "GPU-0",
						MigEnabled: true,
						MigInstances: []topology.MigInstance{
							{Profile: "1g.5gb"},
						},
					},
					{ID: "GPU-1"},
				},
			}
			kubeClient := &fake.Clientset{}
			devicePlugins := NewDevicePlugins(topology, kubeClient)
			Expect(devicePlugins).To(HaveLen(2))

			gpuPlugin, ok := devicePlugins[0].(*RealNodeDevicePlugin)
			Expect(ok).To(BeTrue())
			Expect(gpuPlugin.resourceName).To(Equal("nvidia.com/gpu"))

			migPlugin, ok := devicePlugins[1].(*RealNodeDevicePlugin)
			Expect(ok).To(BeTrue())
			Expect(migPlugin.resourceName).To(Equal("nvidia.com/mig-1g.5gb"))
		})
	})

})

var _ = Describe("resourceSocket", func() {
	It("uses the legacy socket path for nvidia.com/gpu", func() {
		Expect(resourceSocket("nvidia.com/gpu")).To(Equal(serverSock))
	})

	It("uses a fake-prefixed socket for MIG resources", func() {
		Expect(resourceSocket("nvidia.com/mig-1g.5gb")).To(ContainSubstring("fake-nvidia_com_mig_1g_5gb.sock"))
	})
})
