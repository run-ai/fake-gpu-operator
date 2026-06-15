package nrt

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

// MultiNodeExporter publishes NRTs for many KWOK nodes (KWOK / kwok-deployment variant).
type MultiNodeExporter struct {
	kubeClient kubernetes.Interface
	publisher  Publisher
}

func NewMultiNodeExporter(kubeClient kubernetes.Interface, publisher Publisher) *MultiNodeExporter {
	return &MultiNodeExporter{kubeClient: kubeClient, publisher: publisher}
}

// SetNRTForNode publishes the NRT for a specific node.
func (e *MultiNodeExporter) SetNRTForNode(nodeName string, nodeTopology *topology.NodeTopology) error {
	return publishNodeNRT(context.TODO(), e.kubeClient, e.publisher, nodeName, nodeTopology)
}
