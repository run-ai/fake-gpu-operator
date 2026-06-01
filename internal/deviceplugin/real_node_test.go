package deviceplugin

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
)

var _ = Describe("RealNodeDevicePlugin Allocate", func() {
	const nodeName = "worker-1"

	BeforeEach(func() {
		GinkgoT().Setenv(constants.EnvNodeName, nodeName)
	})

	It("injects NODE_NAME, MOCK_NVIDIA_VISIBLE_DEVICES and the nvidia-smi mount", func() {
		m := &RealNodeDevicePlugin{}
		reqs := &pluginapi.AllocateRequest{
			ContainerRequests: []*pluginapi.ContainerAllocateRequest{
				{DevicesIds: []string{"gpu-0", "gpu-1"}},
			},
		}

		resp, err := m.Allocate(context.Background(), reqs)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.ContainerResponses).To(HaveLen(1))

		container := resp.ContainerResponses[0]
		Expect(container.Envs).To(HaveKeyWithValue(constants.EnvNodeName, nodeName))
		Expect(container.Envs).To(HaveKeyWithValue("MOCK_NVIDIA_VISIBLE_DEVICES", "gpu-0,gpu-1"))
		Expect(container.Mounts).To(ContainElement(&pluginapi.Mount{
			ContainerPath: "/bin/nvidia-smi",
			HostPath:      "/var/lib/runai/bin/nvidia-smi",
		}))
	})
})
