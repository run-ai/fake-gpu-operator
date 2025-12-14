package node

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestAnnotateNodeWithTopology(t *testing.T) {
	tests := []struct {
		name          string
		nodeTopology  *topology.NodeTopology
		nodeName      string
		existingAnnot map[string]string
		wantErr       bool
	}{
		{
			name:     "annotate node with basic topology",
			nodeName: "test-node",
			nodeTopology: &topology.NodeTopology{
				GpuMemory:  16000,
				GpuProduct: "NVIDIA-A100",
				Gpus: []topology.GpuDetails{
					{ID: "GPU-0001-0001-0001-0001"},
					{ID: "GPU-0002-0002-0002-0002"},
				},
			},
			existingAnnot: nil,
			wantErr:       false,
		},
		{
			name:     "annotate node with existing annotations",
			nodeName: "test-node",
			nodeTopology: &topology.NodeTopology{
				GpuMemory:  32000,
				GpuProduct: "NVIDIA-H100",
				Gpus: []topology.GpuDetails{
					{ID: "GPU-1111-1111-1111-1111"},
				},
			},
			existingAnnot: map[string]string{
				"existing-annotation": "should-remain",
			},
			wantErr: false,
		},
		{
			name:     "annotate node with allocated GPUs",
			nodeName: "test-node",
			nodeTopology: &topology.NodeTopology{
				GpuMemory:  16000,
				GpuProduct: "NVIDIA-A100",
				Gpus: []topology.GpuDetails{
					{
						ID: "GPU-0001-0001-0001-0001",
						Status: topology.GpuStatus{
							AllocatedBy: topology.ContainerDetails{
								Namespace: "default",
								Pod:       "test-pod",
								Container: "main",
							},
						},
					},
				},
			},
			existingAnnot: nil,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fake node
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        tt.nodeName,
					Annotations: tt.existingAnnot,
				},
			}

			fakeClient := fake.NewClientset(node)

			// Call the function
			err := AnnotateNodeWithTopology(fakeClient, tt.nodeTopology, tt.nodeName)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Verify the annotation was set
			updatedNode, err := fakeClient.CoreV1().Nodes().Get(context.TODO(), tt.nodeName, metav1.GetOptions{})
			require.NoError(t, err)

			annotValue, exists := updatedNode.Annotations[AnnotationGpuFakeDevices]
			assert.True(t, exists, "Annotation should exist")

			// Verify the annotation contains valid JSON that matches our topology
			var parsedTopology topology.NodeTopology
			err = json.Unmarshal([]byte(annotValue), &parsedTopology)
			require.NoError(t, err, "Annotation should contain valid JSON")

			assert.Equal(t, tt.nodeTopology.GpuMemory, parsedTopology.GpuMemory)
			assert.Equal(t, tt.nodeTopology.GpuProduct, parsedTopology.GpuProduct)
			assert.Len(t, parsedTopology.Gpus, len(tt.nodeTopology.Gpus))

			for i, expectedGpu := range tt.nodeTopology.Gpus {
				assert.Equal(t, expectedGpu.ID, parsedTopology.Gpus[i].ID)
			}

			// Verify existing annotations are preserved
			if tt.existingAnnot != nil {
				for k, v := range tt.existingAnnot {
					assert.Equal(t, v, updatedNode.Annotations[k], "Existing annotation should be preserved")
				}
			}
		})
	}
}

func TestAnnotateNodeWithTopology_NodeNotFound(t *testing.T) {
	fakeClient := fake.NewClientset()

	nodeTopology := &topology.NodeTopology{
		GpuMemory:  16000,
		GpuProduct: "NVIDIA-A100",
		Gpus: []topology.GpuDetails{
			{ID: "GPU-0001-0001-0001-0001"},
		},
	}

	err := AnnotateNodeWithTopology(fakeClient, nodeTopology, "non-existent-node")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to patch node")
}
