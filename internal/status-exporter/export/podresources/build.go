package podresources

import (
	"math"
	"sort"

	"k8s.io/apimachinery/pkg/api/resource"
	podresourcesv1 "k8s.io/kubelet/pkg/apis/podresources/v1"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/numazones"
)

// PodRequest is a workload pod's aggregate resource requests (summed across containers).
type PodRequest struct {
	CPU    resource.Quantity
	Memory resource.Quantity
}

type podAgg struct {
	namespace, name string
	gpusByZone      map[int][]string // zone -> device ids
}

// BuildPodResources synthesizes the podresources List response: one entry per workload pod
// that holds fake GPUs, with GPU/cpu/memory charged to NUMA zones. cpu/memory are split
// proportional to per-zone GPU count (largest-remainder). reqs is keyed by "namespace/name".
func BuildPodResources(nt *topology.NodeTopology, layout *numazones.ZoneLayout, reqs map[string]PodRequest) []*podresourcesv1.PodResources {
	if nt == nil || layout == nil {
		return nil
	}
	idxToZone := numazones.GPUIndexToZone(layout.GpusPerZone)

	byPod := map[string]*podAgg{}
	var order []string
	for i, g := range nt.Gpus {
		if g.Status.AllocatedBy.Pod == "" || g.Status.AllocatedBy.Namespace == "" {
			continue
		}
		if i >= len(idxToZone) {
			continue // more GPUs than the layout accounts for; skip defensively
		}
		key := g.Status.AllocatedBy.Namespace + "/" + g.Status.AllocatedBy.Pod
		a := byPod[key]
		if a == nil {
			a = &podAgg{namespace: g.Status.AllocatedBy.Namespace, name: g.Status.AllocatedBy.Pod, gpusByZone: map[int][]string{}}
			byPod[key] = a
			order = append(order, key)
		}
		zone := idxToZone[i]
		a.gpusByZone[zone] = append(a.gpusByZone[zone], g.ID)
	}
	sort.Strings(order)

	out := make([]*podresourcesv1.PodResources, 0, len(order))
	for _, key := range order {
		a := byPod[key]
		zones := make([]int, 0, len(a.gpusByZone))
		weights := []int{}
		for z := range a.gpusByZone {
			zones = append(zones, z)
		}
		sort.Ints(zones)
		for _, z := range zones {
			weights = append(weights, len(a.gpusByZone[z]))
		}

		req := reqs[key]
		cores := int64(math.Round(float64(req.CPU.MilliValue()) / 1000.0))
		coresPerZone := largestRemainder(cores, weights)
		memPerZone := largestRemainder(req.Memory.Value(), weights)

		c := &podresourcesv1.ContainerResources{}
		for zi, z := range zones {
			c.Devices = append(c.Devices, &podresourcesv1.ContainerDevices{
				ResourceName: constants.GpuResourceName,
				DeviceIds:    a.gpusByZone[z],
				Topology:     zoneTopology(z),
			})
			for k := int64(0); k < coresPerZone[zi] && int(k) < len(layout.CPUIDsPerZone[z]); k++ {
				c.CpuIds = append(c.CpuIds, layout.CPUIDsPerZone[z][k])
			}
			if memPerZone[zi] > 0 {
				c.Memory = append(c.Memory, &podresourcesv1.ContainerMemory{
					MemoryType: "memory",
					Size:       uint64(memPerZone[zi]),
					Topology:   zoneTopology(z),
				})
			}
		}
		out = append(out, &podresourcesv1.PodResources{Name: a.name, Namespace: a.namespace, Containers: []*podresourcesv1.ContainerResources{c}})
	}
	return out
}

func zoneTopology(zone int) *podresourcesv1.TopologyInfo {
	return &podresourcesv1.TopologyInfo{Nodes: []*podresourcesv1.NUMANode{{ID: int64(zone)}}}
}

// largestRemainder apportions total across weights, giving leftover units to the largest
// fractional remainders. Returns all-zero when total or sum(weights) is 0.
func largestRemainder(total int64, weights []int) []int64 {
	out := make([]int64, len(weights))
	sum := 0
	for _, w := range weights {
		sum += w
	}
	if sum == 0 || total <= 0 {
		return out
	}
	type rem struct {
		idx  int
		frac float64
	}
	rems := make([]rem, len(weights))
	allocated := int64(0)
	for i, w := range weights {
		exact := float64(total) * float64(w) / float64(sum)
		fl := int64(math.Floor(exact))
		out[i] = fl
		allocated += fl
		rems[i] = rem{i, exact - float64(fl)}
	}
	sort.SliceStable(rems, func(a, b int) bool { return rems[a].frac > rems[b].frac })
	for k := int64(0); k < total-allocated && int(k) < len(rems); k++ {
		out[rems[k].idx]++
	}
	return out
}
