package labels

import (
	"context"
	"fmt"
	"log"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-exporter/watch"
)

// MultiNodeLabelsExporter exports labels for multiple KWOK nodes
type MultiNodeLabelsExporter struct {
	kubeClient kubernetes.Interface
}

var _ watch.LabelsExporter = &MultiNodeLabelsExporter{}

// NewMultiNodeLabelsExporter creates a new multi-node labels exporter
func NewMultiNodeLabelsExporter(kubeClient kubernetes.Interface) *MultiNodeLabelsExporter {
	return &MultiNodeLabelsExporter{
		kubeClient: kubeClient,
	}
}

// SetLabelsForNode exports labels for a specific node
func (e *MultiNodeLabelsExporter) SetLabelsForNode(nodeName string, nodeTopology *topology.NodeTopology) error {
	labels := BuildNodeLabels(nodeTopology)

	if err := e.setNodeLabels(nodeName, labels); err != nil {
		return fmt.Errorf("failed to set node labels for %s: %w", nodeName, err)
	}

	log.Printf("Exported labels for KWOK node: %s\n", nodeName)
	return nil
}

// setNodeLabels sets labels on a specific node with retry logic to handle conflicts
func (e *MultiNodeLabelsExporter) setNodeLabels(nodeName string, labels map[string]string) error {
	log.Printf("Setting labels on KWOK node %s: %v\n", nodeName, labels)

	// Retry on conflict errors (when node is being modified by KWOK stages)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := e.kubeClient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				log.Printf("Node %s not found (may have been deleted)\n", nodeName)
				return nil // Node deleted, don't retry
			}
			return err
		}

		// Update labels
		for k, v := range labels {
			node.Labels[k] = v
		}

		_, err = e.kubeClient.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
		return err
	})
}
