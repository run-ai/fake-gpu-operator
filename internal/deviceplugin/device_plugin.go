package deviceplugin

import (
	"os"
	"path"
	"path/filepath"
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

func NewDevicePlugins(nodeTopology *topology.NodeTopology, kubeClient kubernetes.Interface) []Interface {
	if nodeTopology == nil {
		panic("topology is nil")
	}

	resources := topology.AdvertisedResources(nodeTopology)
	if viper.GetBool(constants.EnvFakeNode) {
		deviceResources := make(map[string]int)
		for _, resource := range resources {
			deviceResources[resource.Name] = resource.Count
		}

		return []Interface{&FakeNodeDevicePlugin{
			kubeClient: kubeClient,
			resources:  deviceResources,
		}}
	}

	devicePlugins := make([]Interface, 0, len(resources))
	for _, resource := range resources {
		devicePlugins = append(devicePlugins, &RealNodeDevicePlugin{
			devs:         createDevices(resource.Count),
			socket:       resourceSocket(resource.Name),
			resourceName: resource.Name,
			stop:         make(chan interface{}),
			health:       make(chan *pluginapi.Device),
		})
	}

	return devicePlugins
}

func CleanupStaleSockets() error {
	sockets, err := filepath.Glob(path.Join(pluginapi.DevicePluginPath, "fake-*.sock"))
	if err != nil {
		return err
	}

	for _, socket := range sockets {
		if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	return nil
}

func resourceSocket(resourceName string) string {
	if resourceName == nvidiaGPUResourceName {
		return serverSock
	}
	return path.Join(pluginapi.DevicePluginPath, "fake-"+normalizeDeviceName(resourceName)+".sock")
}

func normalizeDeviceName(deviceName string) string {
	normalized := strings.ReplaceAll(deviceName, "/", "_")
	normalized = strings.ReplaceAll(normalized, ".", "_")
	normalized = strings.ReplaceAll(normalized, "-", "_")
	return normalized
}
