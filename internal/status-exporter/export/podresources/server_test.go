package podresources

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	podresourcesv1 "k8s.io/kubelet/pkg/apis/podresources/v1"
)

func TestServer_ListReflectsSnapshot(t *testing.T) {
	s := NewServer("/tmp/does-not-matter.sock")

	// Empty before any snapshot.
	resp, err := s.List(context.Background(), &podresourcesv1.ListPodResourcesRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.PodResources)

	s.SetSnapshot([]*podresourcesv1.PodResources{{Name: "p1", Namespace: "ns"}})
	resp, err = s.List(context.Background(), &podresourcesv1.ListPodResourcesRequest{})
	require.NoError(t, err)
	require.Len(t, resp.PodResources, 1)
	assert.Equal(t, "p1", resp.PodResources[0].Name)
}
