package deviceplugin

import (
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/spf13/viper"
	"k8s.io/client-go/kubernetes"
)

const (
	resourceName = "nvidia.com/gpu"
)

type Interface interface {
	Serve() error
}

func NewDevicePlugin(topology *topology.NodeTopology, kubeClient kubernetes.Interface) Interface {
	if topology == nil {
		panic("topology is nil")
	}

	if viper.GetBool(constants.EnvFakeNode) {
		return &FakeNodeDevicePlugin{
			kubeClient: kubeClient,
			gpuCount:   getGpuCount(topology),
		}
	}

	return &RealNodeDevicePlugin{
		devs:   createDevices(getGpuCount(topology)),
		socket: serverSock,
	}
}
