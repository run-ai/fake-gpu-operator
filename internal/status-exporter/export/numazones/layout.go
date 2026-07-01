package numazones

import (
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

// ZoneLayout holds the derived per-node NUMA zone split used by the NRT builder,
// the podresources server, and the sysfs cpulist renderer.
type ZoneLayout struct {
	Zones         int
	GpusPerZone   []int
	CPUPerZone    *resource.Quantity // uniform per-zone; nil => cpu omitted
	MemPerZone    *resource.Quantity // uniform per-zone; nil => memory omitted
	CPUIDsPerZone [][]int64          // per-zone contiguous synthetic core ids
}

// ResolveZoneLayout derives the per-node NUMA zone layout shared by the NRT builder,
// the podresources server, and the sysfs cpulist renderer. Returns (nil, nil) when
// the pool has no NUMA sim (numa.Zones < 1).
func ResolveZoneLayout(numa topology.NumaConfig, gpuCount int, nodeAllocatable corev1.ResourceList) (*ZoneLayout, error) {
	if numa.Zones < 1 {
		return nil, nil
	}
	gpusPerZone, err := distributeGPUs(gpuCount, numa.Zones, numa.GpusPerZone)
	if err != nil {
		return nil, err
	}
	cpuPerZone, err := perZoneCPU(numa, nodeAllocatable, numa.Zones)
	if err != nil {
		return nil, err
	}
	memPerZone, err := perZoneMemory(numa, nodeAllocatable, numa.Zones)
	if err != nil {
		return nil, err
	}

	cores := int64(0)
	if cpuPerZone != nil {
		cores = (cpuPerZone.MilliValue() + 500) / 1000 // round to whole cores
	}
	cpuIDs := make([][]int64, numa.Zones)
	offset := int64(0)
	for z := 0; z < numa.Zones; z++ {
		ids := make([]int64, cores)
		for i := int64(0); i < cores; i++ {
			ids[i] = offset + i
		}
		cpuIDs[z] = ids
		offset += cores
	}

	return &ZoneLayout{
		Zones:         numa.Zones,
		GpusPerZone:   gpusPerZone,
		CPUPerZone:    cpuPerZone,
		MemPerZone:    memPerZone,
		CPUIDsPerZone: cpuIDs,
	}, nil
}

// GPUIndexToZone maps each GPU index to its zone by a cumulative walk over gpusPerZone.
func GPUIndexToZone(gpusPerZone []int) []int {
	total := 0
	for _, n := range gpusPerZone {
		total += n
	}
	out := make([]int, total)
	idx := 0
	for zone, n := range gpusPerZone {
		for i := 0; i < n; i++ {
			out[idx] = zone
			idx++
		}
	}
	return out
}

// Cpulist renders an ascending id slice into Linux cpulist form ("0-3,8").
func Cpulist(ids []int64) string {
	if len(ids) == 0 {
		return ""
	}
	var parts []string
	start, prev := ids[0], ids[0]
	flush := func() {
		if start == prev {
			parts = append(parts, strconv.FormatInt(start, 10))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", start, prev))
		}
	}
	for _, id := range ids[1:] {
		if id == prev+1 {
			prev = id
			continue
		}
		flush()
		start, prev = id, id
	}
	flush()
	return strings.Join(parts, ",")
}

// distributeGPUs splits gpuCount across the given number of zones. If explicit
// per-zone counts are provided they must have one entry per zone and sum to
// gpuCount; otherwise GPUs are split as evenly as possible with the remainder
// assigned to the lowest-indexed zones.
func distributeGPUs(gpuCount, zones int, explicit []int) ([]int, error) {
	if zones < 1 {
		return nil, fmt.Errorf("zones must be >= 1, got %d", zones)
	}
	if len(explicit) > 0 {
		if len(explicit) != zones {
			return nil, fmt.Errorf("gpusPerZone has %d entries but zones is %d", len(explicit), zones)
		}
		sum := 0
		for _, n := range explicit {
			sum += n
		}
		if sum != gpuCount {
			return nil, fmt.Errorf("gpusPerZone sums to %d but the pool has %d GPUs", sum, gpuCount)
		}
		return explicit, nil
	}
	base := gpuCount / zones
	remainder := gpuCount % zones
	out := make([]int, zones)
	for z := range out {
		out[z] = base
		if z < remainder {
			out[z]++
		}
	}
	return out, nil
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
