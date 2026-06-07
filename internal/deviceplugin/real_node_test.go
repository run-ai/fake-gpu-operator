package deviceplugin

import (
	"context"
	"os"
	"path/filepath"

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

var _ = Describe("realNUMACount", func() {
	It("counts node<N> directories (incl. double-digit) and ignores files and non-node entries", func() {
		dir := GinkgoT().TempDir()
		for _, name := range []string{"node0", "node1", "node10", "has_cpu", "online", "power"} {
			Expect(os.Mkdir(filepath.Join(dir, name), 0o755)).To(Succeed())
		}
		// a regular file named like a node dir must NOT be counted (IsDir guard)
		Expect(os.WriteFile(filepath.Join(dir, "node2"), []byte{}, 0o644)).To(Succeed())
		Expect(realNUMACount(dir)).To(Equal(3))
	})

	It("returns 1 when the path is missing", func() {
		Expect(realNUMACount("/nonexistent/sys/node")).To(Equal(1))
	})

	It("returns 1 when there are no node directories", func() {
		dir := GinkgoT().TempDir()
		Expect(os.Mkdir(filepath.Join(dir, "online"), 0o755)).To(Succeed())
		Expect(realNUMACount(dir)).To(Equal(1))
	})
})
