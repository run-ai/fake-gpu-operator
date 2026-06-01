package topology

import (
	"sort"
	"strings"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
)

const migResourcePrefix = "nvidia.com/mig-"

// MigResourceName returns the Kubernetes extended resource name for a MIG
// profile, for example "1g.5gb" -> "nvidia.com/mig-1g.5gb".
func MigResourceName(profile string) string {
	return migResourcePrefix + profile
}

// AdvertisedResources returns the resources that should be exposed to the
// Kubernetes scheduler for this node topology.
func AdvertisedResources(nodeTopology *NodeTopology) []GenericDevice {
	if nodeTopology == nil {
		return nil
	}

	counts := map[string]int{}
	switch strings.ToLower(nodeTopology.MigStrategy) {
	case "mixed":
		for _, gpu := range nodeTopology.Gpus {
			if gpu.MigEnabled {
				for _, migInstance := range gpu.MigInstances {
					if migInstance.Profile == "" {
						continue
					}
					counts[MigResourceName(migInstance.Profile)]++
				}
				continue
			}
			counts[constants.GpuResourceName]++
		}
	case "single":
		count := 0
		for _, gpu := range nodeTopology.Gpus {
			if gpu.MigEnabled {
				count += len(gpu.MigInstances)
				continue
			}
			count++
		}
		counts[constants.GpuResourceName] = count
	default:
		counts[constants.GpuResourceName] = len(nodeTopology.Gpus)
	}

	for _, device := range nodeTopology.OtherDevices {
		if device.Name == "" || device.Count <= 0 {
			continue
		}
		counts[device.Name] += device.Count
	}

	resources := make([]GenericDevice, 0, len(counts))
	for name, count := range counts {
		if count <= 0 && name != constants.GpuResourceName {
			continue
		}
		resources = append(resources, GenericDevice{Name: name, Count: count})
	}

	sort.Slice(resources, func(i, j int) bool {
		if resources[i].Name == constants.GpuResourceName {
			return true
		}
		if resources[j].Name == constants.GpuResourceName {
			return false
		}
		return resources[i].Name < resources[j].Name
	})

	return resources
}
