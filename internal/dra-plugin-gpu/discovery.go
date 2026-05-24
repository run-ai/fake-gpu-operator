package dra_plugin_gpu

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
)

const (
	// topologyServerURL is the base URL for the topology server
	topologyServerURL = "http://topology-server.gpu-operator/topology/nodes/"
)

// getTopologyFromHTTP retrieves node topology from the HTTP topology server
func getTopologyFromHTTP(nodeName string) (*topology.NodeTopology, error) {
	resp, err := http.Get(topologyServerURL + nodeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get topology from HTTP server: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("topology server returned status %d for node %s", resp.StatusCode, nodeName)
	}

	var nodeTopology topology.NodeTopology
	if err := json.NewDecoder(resp.Body).Decode(&nodeTopology); err != nil {
		return nil, fmt.Errorf("failed to decode topology response: %w", err)
	}

	return &nodeTopology, nil
}

func enumerateAllPossibleDevices(nodeName string) (AllocatableDevices, error) {
	// Get topology from HTTP server
	nodeTopology, err := getTopologyFromHTTP(nodeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get topology for node %s: %w", nodeName, err)
	}

	if len(nodeTopology.Gpus) == 0 {
		return nil, fmt.Errorf("topology server returned no GPUs for node %s", nodeName)
	}

	// Convert GpuMemory from MB to bytes for resource.Quantity
	memoryBytes := int64(nodeTopology.GpuMemory) * 1024 * 1024

	// Map GPU info to resourceapi.Device structures
	alldevices := make(AllocatableDevices)
	for _, gpu := range nodeTopology.Gpus {
		if gpu.ID == "" {
			return nil, fmt.Errorf("GPU entry missing ID in topology")
		}

		// Use ID (UUID) as device name, convert to lowercase for RFC 1123 compliance
		deviceName := strings.ToLower(gpu.ID)

		// Attributes under the `gpu.nvidia.com` domain satisfy upstream
		// nvidia-dra-driver-gpu's DeviceClass CEL selector
		// (`device.attributes['gpu.nvidia.com'].type == 'gpu'`). DRA addresses
		// `<domain>/<name>` qualified keys as `attributes[<domain>].<name>`
		// in CEL, so the keys MUST carry the `gpu.nvidia.com/` prefix —
		// unqualified `type` would resolve to a different namespace and
		// fail the same selector. Unqualified `uuid` and `model` are kept
		// for backwards compatibility with any consumer reading them.
		attributes := map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
			"uuid": {
				StringValue: ptr.To(gpu.ID),
			},
			"model": {
				StringValue: ptr.To(nodeTopology.GpuProduct),
			},
			"gpu.nvidia.com/type": {
				StringValue: ptr.To("gpu"),
			},
			"gpu.nvidia.com/uuid": {
				StringValue: ptr.To(gpu.ID),
			},
			"gpu.nvidia.com/productName": {
				StringValue: ptr.To(nodeTopology.GpuProduct),
			},
		}

		// Convert memory to resource.Quantity
		memoryQuantity := resource.NewQuantity(memoryBytes, resource.BinarySI)

		device := resourceapi.Device{
			Name:       deviceName,
			Attributes: attributes,
			Capacity: map[resourceapi.QualifiedName]resourceapi.DeviceCapacity{
				"memory": {
					Value: *memoryQuantity,
				},
			},
		}
		alldevices[device.Name] = device
	}

	return alldevices, nil
}
