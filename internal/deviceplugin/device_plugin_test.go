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
	})

})
