package nrt

import "fmt"

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
