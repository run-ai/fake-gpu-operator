package nrt

import (
	"fmt"

	v1alpha2 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

const (
	apiVersion        = "topology.node.k8s.io/v1alpha2"
	kind              = "NodeResourceTopology"
	gpuResourceName   = "nvidia.com/gpu"
	zoneType          = "Node"
	defaultPolicy     = "single-numa-node"
	defaultScope      = "container"
	defaultSelfCost   = 10
	defaultRemoteCost = 21
)

// BuildNRT constructs a NodeResourceTopology for a node from its pool's declared
// NUMA layout and GPU count. It distributes the GPUs across the configured zones,
// derives per-zone cpu/memory (from the numa overrides or by splitting node
// allocatable), and emits zone distance costs plus Topology-Manager attributes.
// In R1 available == allocatable == capacity (static); R2 makes available live.
// Returns (nil, nil) when numa.Zones < 1 (publishing disabled for the pool).
func BuildNRT(nodeName string, numa topology.NumaConfig, gpuCount int, nodeAllocatable corev1.ResourceList) (*v1alpha2.NodeResourceTopology, error) {
	if numa.Zones < 1 {
		return nil, nil
	}

	gpusPerZone, err := distributeGPUs(gpuCount, numa.Zones, numa.GpusPerZone)
	if err != nil {
		return nil, err
	}

	policy := numa.TopologyManagerPolicy
	if policy == "" {
		policy = defaultPolicy
	}
	scope := numa.TopologyManagerScope
	if scope == "" {
		scope = defaultScope
	}

	self, remote := defaultSelfCost, defaultRemoteCost
	if numa.Distances != nil {
		self, remote = numa.Distances.Self, numa.Distances.Remote
	}

	cpuPerZone, err := perZoneCPU(numa, nodeAllocatable, numa.Zones)
	if err != nil {
		return nil, err
	}
	memPerZone, err := perZoneMemory(numa, nodeAllocatable, numa.Zones)
	if err != nil {
		return nil, err
	}

	zones := make(v1alpha2.ZoneList, numa.Zones)
	for z := 0; z < numa.Zones; z++ {
		costs := make(v1alpha2.CostList, numa.Zones)
		for o := 0; o < numa.Zones; o++ {
			value := int64(remote)
			if o == z {
				value = int64(self)
			}
			costs[o] = v1alpha2.CostInfo{Name: zoneName(o), Value: value}
		}

		resources := v1alpha2.ResourceInfoList{
			resourceInfo(gpuResourceName, *resource.NewQuantity(int64(gpusPerZone[z]), resource.DecimalSI)),
		}
		if cpuPerZone != nil {
			resources = append(resources, resourceInfo("cpu", *cpuPerZone))
		}
		if memPerZone != nil {
			resources = append(resources, resourceInfo("memory", *memPerZone))
		}

		zones[z] = v1alpha2.Zone{
			Name:      zoneName(z),
			Type:      zoneType,
			Costs:     costs,
			Resources: resources,
		}
	}

	return &v1alpha2.NodeResourceTopology{
		TypeMeta:   metav1.TypeMeta{APIVersion: apiVersion, Kind: kind},
		ObjectMeta: metav1.ObjectMeta{Name: nodeName},
		Attributes: v1alpha2.AttributeList{
			{Name: "topologyManagerPolicy", Value: policy},
			{Name: "topologyManagerScope", Value: scope},
		},
		TopologyPolicies: []string{legacyTopologyPolicy(policy, scope)},
		Zones:            zones,
	}, nil
}

func zoneName(i int) string { return fmt.Sprintf("node-%d", i) }

// resourceInfo builds a static ResourceInfo where capacity == allocatable == available.
func resourceInfo(name string, q resource.Quantity) v1alpha2.ResourceInfo {
	return v1alpha2.ResourceInfo{
		Name:        name,
		Capacity:    q,
		Allocatable: q,
		Available:   q,
	}
}

// perZoneCPU returns the per-zone cpu quantity: the explicit override if set,
// else node-allocatable cpu divided by the zone count, else nil (cpu omitted).
// Integer division truncates; any remainder is intentionally dropped (the
// per-zone totals may sum to slightly under node allocatable — fine for a sim).
func perZoneCPU(numa topology.NumaConfig, alloc corev1.ResourceList, zones int) (*resource.Quantity, error) {
	if numa.CPUPerZone != "" {
		q, err := resource.ParseQuantity(numa.CPUPerZone)
		if err != nil {
			return nil, fmt.Errorf("invalid cpuPerZone %q: %w", numa.CPUPerZone, err)
		}
		return &q, nil
	}
	cpu := alloc.Cpu()
	if cpu.IsZero() {
		return nil, nil
	}
	return resource.NewMilliQuantity(cpu.MilliValue()/int64(zones), resource.DecimalSI), nil
}

// perZoneMemory mirrors perZoneCPU for memory (binary SI), same truncation.
func perZoneMemory(numa topology.NumaConfig, alloc corev1.ResourceList, zones int) (*resource.Quantity, error) {
	if numa.MemPerZone != "" {
		q, err := resource.ParseQuantity(numa.MemPerZone)
		if err != nil {
			return nil, fmt.Errorf("invalid memPerZone %q: %w", numa.MemPerZone, err)
		}
		return &q, nil
	}
	mem := alloc.Memory()
	if mem.IsZero() {
		return nil, nil
	}
	return resource.NewQuantity(mem.Value()/int64(zones), resource.BinarySI), nil
}

// legacyTopologyPolicy maps the modern policy/scope pair to the deprecated
// TopologyPolicies enum some consumers still read.
func legacyTopologyPolicy(policy, scope string) string {
	level := "ContainerLevel"
	if scope == "pod" {
		level = "PodLevel"
	}
	switch policy {
	case "single-numa-node":
		return "SingleNUMANode" + level
	case "restricted":
		return "Restricted" + level
	case "best-effort":
		return "BestEffort" + level
	default:
		return "None"
	}
}
