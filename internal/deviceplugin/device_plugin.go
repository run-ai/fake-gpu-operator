package deviceplugin

import (
	"fmt"
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

func NewDevicePlugins(topology *topology.NodeTopology, kubeClient kubernetes.Interface) ([]Interface, error) {
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
		}}, nil
	}

	gpuDevs, err := createDevices(getGpuCount(topology))
	if err != nil {
		return nil, fmt.Errorf("failed to create GPU devices: %w", err)
	}

	devicePlugins := []Interface{
		&RealNodeDevicePlugin{
			devs:         gpuDevs,
			socket:       serverSock,
			resourceName: nvidiaGPUResourceName,
		},
	}

	for _, genericDevice := range topology.OtherDevices {
		otherDevs, err := createDevices(genericDevice.Count)
		if err != nil {
			return nil, fmt.Errorf("failed to create devices for %s: %w", genericDevice.Name, err)
		}
		devicePlugins = append(devicePlugins, &RealNodeDevicePlugin{
			devs:         otherDevs,
			socket:       path.Join(pluginapi.DevicePluginPath, normalizeDeviceName(genericDevice.Name)+".sock"),
			resourceName: genericDevice.Name,
		})
	}

	return devicePlugins, nil
}

func normalizeDeviceName(deviceName string) string {
	normalized := strings.ReplaceAll(deviceName, "/", "_")
	normalized = strings.ReplaceAll(normalized, ".", "_")
	normalized = strings.ReplaceAll(normalized, "-", "_")
	return normalized
}
