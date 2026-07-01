package podresources

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/apimachinery/pkg/api/resource"
	podresourcesv1 "k8s.io/kubelet/pkg/apis/podresources/v1"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/export/numazones"
)

// TestServer_RealSocketRoundTrip exercises the actual wire path a real KAI npe uses:
// it serves BuildPodResources output over a real unix socket and dials it with the
// generated PodResourcesLister client, asserting the List response survives the round trip.
func TestServer_RealSocketRoundTrip(t *testing.T) {
	// Short path: unix socket sun_path is capped (~104 bytes on macOS), so t.TempDir() is too long.
	dir, err := os.MkdirTemp("/tmp", "fgopr")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(dir) }()
	sock := filepath.Join(dir, "k.sock")
	srv := NewServer(sock)
	require.NoError(t, srv.Start())
	defer srv.Stop()

	cpu := resource.MustParse("8")
	mem := resource.MustParse("16Gi")
	layout := &numazones.ZoneLayout{
		Zones: 2, GpusPerZone: []int{4, 4},
		CPUPerZone: &cpu, MemPerZone: &mem,
		CPUIDsPerZone: [][]int64{{0, 1, 2, 3, 4, 5, 6, 7}, {8, 9, 10, 11, 12, 13, 14, 15}},
	}
	nt := &topology.NodeTopology{Gpus: []topology.GpuDetails{
		{ID: "g0", Status: topology.GpuStatus{AllocatedBy: topology.ContainerDetails{Namespace: "team", Pod: "p1"}}},
		{ID: "g1", Status: topology.GpuStatus{AllocatedBy: topology.ContainerDetails{Namespace: "team", Pod: "p1"}}},
	}}
	reqs := map[string]PodRequest{"team/p1": {CPU: resource.MustParse("4"), Memory: resource.MustParse("8Gi")}}
	srv.SetSnapshot(BuildPodResources(nt, layout, reqs))

	conn, err := grpc.NewClient("unix://"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	client := podresourcesv1.NewPodResourcesListerClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := client.List(ctx, &podresourcesv1.ListPodResourcesRequest{})
	require.NoError(t, err)

	require.Len(t, resp.PodResources, 1)
	pr := resp.PodResources[0]
	assert.Equal(t, "p1", pr.Name)
	assert.Equal(t, "team", pr.Namespace)
	require.Len(t, pr.Containers, 1)
	c := pr.Containers[0]
	require.Len(t, c.Devices, 1, "both GPUs are in zone 0 -> one ContainerDevices")
	assert.Equal(t, "nvidia.com/gpu", c.Devices[0].ResourceName)
	assert.Len(t, c.Devices[0].DeviceIds, 2)
	require.Len(t, c.Devices[0].Topology.Nodes, 1)
	assert.Equal(t, int64(0), c.Devices[0].Topology.Nodes[0].ID)
	assert.Equal(t, []int64{0, 1, 2, 3}, c.CpuIds)
	require.Len(t, c.Memory, 1)
	wantMem := resource.MustParse("8Gi")
	assert.Equal(t, uint64(wantMem.Value()), c.Memory[0].Size)

	// Get/GetAllocatableResources must be Unimplemented (List-only server).
	_, err = client.GetAllocatableResources(ctx, &podresourcesv1.AllocatableResourcesRequest{})
	assert.Error(t, err, "GetAllocatableResources should be Unimplemented")
}
