package nrt

import (
	"context"
	"testing"

	v1alpha2 "github.com/k8stopologyawareschedwg/noderesourcetopology-api/pkg/apis/topology/v1alpha2"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

type recordingPublisher struct {
	applied []*v1alpha2.NodeResourceTopology
}

func (p *recordingPublisher) Apply(_ context.Context, nrt *v1alpha2.NodeResourceTopology) error {
	p.applied = append(p.applied, nrt)
	return nil
}

func seedCluster(t *testing.T, clusterYAML string, node *corev1.Node) *fake.Clientset {
	t.Helper()
	viper.Set(constants.EnvTopologyCmNamespace, "gpu-operator")
	viper.Set(constants.EnvTopologyCmName, "topology")
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "topology", Namespace: "gpu-operator"},
		Data:       map[string]string{topology.CmTopologyKey: clusterYAML},
	}
	return fake.NewSimpleClientset(cm, node)
}

func gpuNode(name, pool string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			UID:    types.UID("uid-" + name),
			Labels: map[string]string{"run.ai/simulated-gpu-node-pool": pool},
		},
		Status: corev1.NodeStatus{Allocatable: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("8"),
			corev1.ResourceMemory: resource.MustParse("128Gi"),
		}},
	}
}

const numaClusterYAML = `
nodePoolLabelKey: run.ai/simulated-gpu-node-pool
nodePools:
  default:
    gpu: {backend: fake, profile: a100}
    numa: {zones: 2, distances: {self: 10, remote: 21}}
`

func topo(gpuCount int) *topology.NodeTopology {
	gpus := make([]topology.GpuDetails, gpuCount)
	return &topology.NodeTopology{Gpus: gpus}
}

func TestPublishNodeNRT_PublishesWithOwnerRef(t *testing.T) {
	client := seedCluster(t, numaClusterYAML, gpuNode("node-a", "default"))
	pub := &recordingPublisher{}

	require.NoError(t, publishNodeNRT(context.Background(), client, pub, "node-a", topo(8)))

	require.Len(t, pub.applied, 1)
	got := pub.applied[0]
	assert.Equal(t, "node-a", got.Name)
	require.Len(t, got.Zones, 2)
	require.Len(t, got.OwnerReferences, 1)
	assert.Equal(t, "Node", got.OwnerReferences[0].Kind)
	assert.Equal(t, "node-a", got.OwnerReferences[0].Name)
	assert.Equal(t, types.UID("uid-node-a"), got.OwnerReferences[0].UID)
}

func TestPublishNodeNRT_SkipsWhenPoolHasNoNuma(t *testing.T) {
	const noNuma = `
nodePoolLabelKey: run.ai/simulated-gpu-node-pool
nodePools:
  default:
    gpu: {backend: fake, profile: a100}
`
	client := seedCluster(t, noNuma, gpuNode("node-a", "default"))
	pub := &recordingPublisher{}

	require.NoError(t, publishNodeNRT(context.Background(), client, pub, "node-a", topo(8)))
	assert.Empty(t, pub.applied)
}

func TestPublishNodeNRT_SkipsWhenNodeNotInAPool(t *testing.T) {
	node := gpuNode("node-a", "default")
	node.Labels = nil
	client := seedCluster(t, numaClusterYAML, node)
	pub := &recordingPublisher{}

	require.NoError(t, publishNodeNRT(context.Background(), client, pub, "node-a", topo(8)))
	assert.Empty(t, pub.applied)
}
