package deviceplugin

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	serverSock = pluginapi.DevicePluginPath + "fake-nvidia-gpu.sock"

	// sysDevicesSystemNodePath is the per-NUMA-node sysfs dir, under the host /sys that the
	// Helm chart mounts read-only at /host-sys. Mounting all of /sys (which always exists)
	// rather than the leaf node dir lets the device-plugin pod start even when this subpath
	// is absent (e.g. KIND / non-NUMA sysfs); realNUMACount then falls back to 1.
	sysDevicesSystemNodePath = "/host-sys/devices/system/node"
)

var numaNodeDirRe = regexp.MustCompile(`^node[0-9]+$`)

// realNUMACount counts node<N> directories under the given sysfs node path.
// It returns 1 when the path is missing, unreadable, or contains no node dirs,
// so NUMA reporting degrades gracefully on nodes without NUMA sysfs.
func realNUMACount(sysNodePath string) int {
	entries, err := os.ReadDir(sysNodePath)
	if err != nil {
		log.Printf("realNUMACount: cannot read %s, defaulting to 1 NUMA node: %v", sysNodePath, err)
		return 1
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() && numaNodeDirRe.MatchString(e.Name()) {
			count++
		}
	}
	if count == 0 {
		return 1
	}
	return count
}

type RealNodeDevicePlugin struct {
	pluginapi.UnimplementedDevicePluginServer
	devs   []*pluginapi.Device
	socket string

	stop   chan interface{}
	health chan *pluginapi.Device
	server *grpc.Server

	resourceName string
}

func getGpuCount(nodeTopology *topology.NodeTopology) int {
	return len(nodeTopology.Gpus)
}

func (m *RealNodeDevicePlugin) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

func dial(unixSocketPath string, timeout time.Duration) (*grpc.ClientConn, error) {
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	//nolint:staticcheck // grpc.DialContext and WithBlock are deprecated but supported throughout 1.x
	c, err := grpc.DialContext(
		ctx,
		unixSocketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), //nolint:staticcheck
		grpc.WithContextDialer(func(_ context.Context, addr string) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)

	if err != nil {
		return nil, err
	}

	return c, nil
}

func createDevices(devCount int, gpus []topology.GpuDetails, realNUMA int) []*pluginapi.Device {
	var devs []*pluginapi.Device
	for i := 0; i < devCount; i++ {
		u, _ := uuid.NewRandom()
		dev := &pluginapi.Device{
			ID:     u.String(),
			Health: pluginapi.Healthy,
		}
		// gpus is nil for non-GPU (OtherDevices) resources, and devCount == len(gpus)
		// for GPUs by construction, so i < len(gpus) only sets topology for real GPUs.
		// NUMANode >= 0 skips the sentinel/negative case (e.g. -1 = no NUMA); realNUMA > 0
		// guards the modulo.
		if i < len(gpus) && gpus[i].NUMANode != nil && *gpus[i].NUMANode >= 0 && realNUMA > 0 {
			dev.Topology = &pluginapi.TopologyInfo{
				Nodes: []*pluginapi.NUMANode{
					{ID: int64(*gpus[i].NUMANode % realNUMA)},
				},
			}
		}
		devs = append(devs, dev)
	}
	return devs
}

func (m *RealNodeDevicePlugin) Start() error {
	err := m.cleanup()
	if err != nil {
		return err
	}

	sock, err := net.Listen("unix", m.socket)
	if err != nil {
		return err
	}

	m.server = grpc.NewServer([]grpc.ServerOption{}...)
	pluginapi.RegisterDevicePluginServer(m.server, m)

	go func() {
		err := m.server.Serve(sock)
		if err != nil {
			log.Println(err)
		}
	}()

	// Wait for server to start by launching a blocking connexion
	conn, err := dial(m.socket, 5*time.Second)
	if err != nil {
		return err
	}
	if err := conn.Close(); err != nil {
		return fmt.Errorf("error closing connection: %v", err)
	}

	return nil
}

func (m *RealNodeDevicePlugin) Stop() error {
	if m.server == nil {
		return nil
	}

	m.server.Stop()
	m.server = nil
	close(m.stop)

	return m.cleanup()
}

func (m *RealNodeDevicePlugin) Register(kubeletEndpoint string) error {
	conn, err := dial(kubeletEndpoint, 5*time.Second)
	if err != nil {
		return err
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Error closing connection: %v\n", err)
		}
	}()

	client := pluginapi.NewRegistrationClient(conn)
	reqt := &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     path.Base(m.socket),
		ResourceName: m.resourceName,
	}

	_, err = client.Register(context.Background(), reqt)
	if err != nil {
		return err
	}
	return nil
}

func (m *RealNodeDevicePlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	err := s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devs})
	if err != nil {
		fmt.Printf("Failed to send devices to Kubelet: %v\n", err)
	}

	for {
		select {
		case <-m.stop:
			return nil
		case d := <-m.health:
			// FIXME: there is no way to recover from the Unhealthy state.
			d.Health = pluginapi.Unhealthy
			err := s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devs})
			if err != nil {
				log.Printf("failed to send unhealthy update: %v", err)
			}
		}
	}
}

func (m *RealNodeDevicePlugin) GetPreferredAllocation(context.Context, *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	return &pluginapi.PreferredAllocationResponse{}, nil
}

func (m *RealNodeDevicePlugin) Allocate(ctx context.Context, reqs *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	responses := pluginapi.AllocateResponse{}
	for _, req := range reqs.ContainerRequests {
		response := pluginapi.ContainerAllocateResponse{
			Envs: map[string]string{
				"MOCK_NVIDIA_VISIBLE_DEVICES": strings.Join(req.DevicesIds, ","),
				// Propagate NODE_NAME (from the DaemonSet's downward API) so the workload's
				// nvidia-smi can resolve its node topology.
				constants.EnvNodeName: os.Getenv(constants.EnvNodeName),
			},
			Mounts: []*pluginapi.Mount{
				{
					ContainerPath: "/bin/nvidia-smi",
					HostPath:      "/var/lib/runai/bin/nvidia-smi",
				},
			},
		}

		responses.ContainerResponses = append(responses.ContainerResponses, &response)
	}

	return &responses, nil
}

func (m *RealNodeDevicePlugin) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (m *RealNodeDevicePlugin) cleanup() error {
	if err := os.Remove(m.socket); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (m *RealNodeDevicePlugin) Serve() error {
	err := m.Start()
	if err != nil {
		log.Printf("Could not start device plugin: %s", err)
		return err
	}
	log.Println("Starting to serve on", m.socket)

	err = m.Register(pluginapi.KubeletSocket)
	if err != nil {
		log.Printf("Could not register device plugin: %s", err)
		stopErr := m.Stop()
		if stopErr != nil {
			log.Printf("Could not stop device plugin: %s", stopErr)
		}
		return err
	}
	log.Println("Registered device plugin with Kubelet")

	return nil
}

func (m *RealNodeDevicePlugin) Name() string {
	return "RealNodeDevicePlugin-" + m.resourceName
}
