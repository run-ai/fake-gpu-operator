package topology

import "math"

func (m *PodGpuUsageStatusMap) Utilization() int {
	var sum int
	for _, v := range *m {
		sum += v.Utilization.Random()
	}

	return int(math.Min(100, float64(sum)))
}

func (m *PodGpuUsageStatusMap) FbUsed(fbTotal int) int {
	var sum int
	for _, v := range *m {
		sum += v.FbUsed
	}

	return int(math.Min(float64(fbTotal), float64(sum)))
}
