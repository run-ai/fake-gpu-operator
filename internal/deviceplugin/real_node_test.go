package deviceplugin

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
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

var _ = Describe("createDevices", func() {
	numa := func(n int) *int { return &n }

	It("clamps profile NUMA onto the real NUMA count and sets Topology per index", func() {
		gpus := []topology.GpuDetails{
			{ID: "g0", NUMANode: numa(0)},
			{ID: "g1", NUMANode: numa(1)},
			{ID: "g2", NUMANode: numa(1)},
		}
		// realNUMA=1 -> every GPU clamps to node 0
		devs := createDevices(len(gpus), gpus, 1)
		Expect(devs).To(HaveLen(3))
		for _, d := range devs {
			Expect(d.Topology).NotTo(BeNil())
			Expect(d.Topology.Nodes).To(HaveLen(1))
			Expect(d.Topology.Nodes[0].ID).To(Equal(int64(0)))
		}

		// realNUMA=2 -> NUMA preserved
		devs = createDevices(len(gpus), gpus, 2)
		Expect(devs[0].Topology.Nodes[0].ID).To(Equal(int64(0)))
		Expect(devs[1].Topology.Nodes[0].ID).To(Equal(int64(1)))
		Expect(devs[2].Topology.Nodes[0].ID).To(Equal(int64(1)))
	})

	It("leaves Topology nil when a GPU has no NUMANode", func() {
		gpus := []topology.GpuDetails{{ID: "g0"}}
		devs := createDevices(1, gpus, 2)
		Expect(devs).To(HaveLen(1))
		Expect(devs[0].Topology).To(BeNil())
	})

	It("leaves Topology nil for non-GPU devices (nil gpus slice)", func() {
		devs := createDevices(2, nil, 2)
		Expect(devs).To(HaveLen(2))
		Expect(devs[0].Topology).To(BeNil())
		Expect(devs[1].Topology).To(BeNil())
	})

	It("wraps a profile NUMA value >= realNUMA via modulo", func() {
		gpus := []topology.GpuDetails{{ID: "g0", NUMANode: numa(3)}}
		devs := createDevices(1, gpus, 2)
		Expect(devs[0].Topology).NotTo(BeNil())
		Expect(devs[0].Topology.Nodes[0].ID).To(Equal(int64(1))) // 3 % 2 == 1
	})

	It("leaves Topology nil for a negative NUMANode sentinel", func() {
		gpus := []topology.GpuDetails{{ID: "g0", NUMANode: numa(-1)}}
		devs := createDevices(1, gpus, 2)
		Expect(devs[0].Topology).To(BeNil())
	})
})
