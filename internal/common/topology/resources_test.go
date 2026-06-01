package topology

import (
	"testing"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
)

func TestAdvertisedResourcesNoneStrategy(t *testing.T) {
	nodeTopology := &NodeTopology{
		MigStrategy: "none",
		Gpus: []GpuDetails{
			{ID: "GPU-0"},
			{ID: "GPU-1"},
		},
		OtherDevices: []GenericDevice{
			{Name: "rdma/hca", Count: 2},
		},
	}

	resources := AdvertisedResources(nodeTopology)
	if len(resources) != 2 {
		t.Fatalf("AdvertisedResources length = %d, want 2", len(resources))
	}
	if resources[0] != (GenericDevice{Name: constants.GpuResourceName, Count: 2}) {
		t.Fatalf("GPU resource = %+v, want nvidia.com/gpu=2", resources[0])
	}
	if resources[1] != (GenericDevice{Name: "rdma/hca", Count: 2}) {
		t.Fatalf("other resource = %+v, want rdma/hca=2", resources[1])
	}
}

func TestAdvertisedResourcesMixedStrategy(t *testing.T) {
	nodeTopology := &NodeTopology{
		MigStrategy: "mixed",
		Gpus: []GpuDetails{
			{
				ID:         "GPU-0",
				MigEnabled: true,
				MigInstances: []MigInstance{
					{Profile: "1g.5gb"},
					{Profile: "1g.5gb"},
					{Profile: "2g.10gb"},
				},
			},
			{ID: "GPU-1"},
		},
	}

	resources := AdvertisedResources(nodeTopology)
	if len(resources) != 3 {
		t.Fatalf("AdvertisedResources length = %d, want 3", len(resources))
	}
	if resources[0] != (GenericDevice{Name: constants.GpuResourceName, Count: 1}) {
		t.Fatalf("GPU resource = %+v, want nvidia.com/gpu=1", resources[0])
	}
	if resources[1] != (GenericDevice{Name: "nvidia.com/mig-1g.5gb", Count: 2}) {
		t.Fatalf("first MIG resource = %+v, want nvidia.com/mig-1g.5gb=2", resources[1])
	}
	if resources[2] != (GenericDevice{Name: "nvidia.com/mig-2g.10gb", Count: 1}) {
		t.Fatalf("second MIG resource = %+v, want nvidia.com/mig-2g.10gb=1", resources[2])
	}
}

func TestAdvertisedResourcesSingleStrategy(t *testing.T) {
	// In "single" strategy, MIG slices are advertised as plain nvidia.com/gpu
	// (count = number of slices), not as nvidia.com/mig-<profile>.
	nodeTopology := &NodeTopology{
		MigStrategy: "single",
		Gpus: []GpuDetails{
			{
				ID:         "GPU-0",
				MigEnabled: true,
				MigInstances: []MigInstance{
					{Profile: "1g.5gb"},
					{Profile: "1g.5gb"},
					{Profile: "1g.5gb"},
				},
			},
			{ID: "GPU-1"},
		},
	}

	resources := AdvertisedResources(nodeTopology)
	if len(resources) != 1 {
		t.Fatalf("AdvertisedResources length = %d, want 1", len(resources))
	}
	// 3 slices on GPU-0 + 1 whole GPU-1 = 4 generic GPUs.
	if resources[0] != (GenericDevice{Name: constants.GpuResourceName, Count: 4}) {
		t.Fatalf("GPU resource = %+v, want nvidia.com/gpu=4", resources[0])
	}
}

func TestAdvertisedResourcesMixedAllMigOmitsGpu(t *testing.T) {
	nodeTopology := &NodeTopology{
		MigStrategy: "mixed",
		Gpus: []GpuDetails{
			{
				ID:         "GPU-0",
				MigEnabled: true,
				MigInstances: []MigInstance{
					{Profile: "1g.5gb"},
				},
			},
		},
	}

	resources := AdvertisedResources(nodeTopology)
	if len(resources) != 1 {
		t.Fatalf("AdvertisedResources length = %d, want 1", len(resources))
	}
	if resources[0] != (GenericDevice{Name: "nvidia.com/mig-1g.5gb", Count: 1}) {
		t.Fatalf("MIG resource = %+v, want nvidia.com/mig-1g.5gb=1", resources[0])
	}
}
