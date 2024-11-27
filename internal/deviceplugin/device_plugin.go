package deviceplugin

import (
	"path"
	"strings"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/spf13/viper"
	"k8s.io/client-go/kubernetes"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

const (
	nvidiaGPUResourceName = "nvidia.com/gpu"
)

type Interface interface {
	Serve() error
	Name() string
}

func NewDevicePlugins(topology *topology.NodeTopology, kubeClient kubernetes.Interface) []Interface {
	if topology == nil {
		panic("topology is nil")
	}

	if viper.GetBool(constants.EnvFakeNode) {
		otherDevices := make(map[string]int)
		for _, genericDevice := range topology.OtherDevices {
			otherDevices[genericDevice.Name] = genericDevice.Count
		}

		return []Interface{&FakeNodeDevicePlugin{
			kubeClient:   kubeClient,
			gpuCount:     getGpuCount(topology),
			otherDevices: otherDevices,
		}}
	}

	devicePlugins := []Interface{
		&RealNodeDevicePlugin{
			devs:         createDevices(getGpuCount(topology)),
			socket:       serverSock,
			resourceName: nvidiaGPUResourceName,
		},
	}

	for _, genericDevice := range topology.OtherDevices {
		devicePlugins = append(devicePlugins, &RealNodeDevicePlugin{
			devs:         createDevices(genericDevice.Count),
			socket:       path.Join(pluginapi.DevicePluginPath, normalizeDeviceName(genericDevice.Name)+".sock"),
			resourceName: genericDevice.Name,
		})
	}

	return devicePlugins
}

func normalizeDeviceName(deviceName string) string {
	normalized := strings.ReplaceAll(deviceName, "/", "_")
	normalized = strings.ReplaceAll(normalized, ".", "_")
	normalized = strings.ReplaceAll(normalized, "-", "_")
	return normalized
}
