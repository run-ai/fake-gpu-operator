package nrt

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

// publishNodeNRT resolves a node's pool numa layout, builds a static NRT, stamps
// an owner reference to the Node (so the NRT is garbage-collected when the node
// is deleted), and applies it. Nodes whose pool has no numa block are skipped.
// Shared by the single-node (DaemonSet) and MultiNode (KWOK) exporters.
func publishNodeNRT(ctx context.Context, kubeClient kubernetes.Interface, publisher Publisher, nodeName string, nodeTopology *topology.NodeTopology) error {
	clusterConfig, err := topology.GetClusterConfigFromCM(kubeClient)
	if err != nil {
		return fmt.Errorf("failed to get cluster config: %w", err)
	}

	node, err := kubeClient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node %s: %w", nodeName, err)
	}

	poolName, ok := node.Labels[clusterConfig.NodePoolLabelKey]
	if !ok {
		return nil
	}
	poolConfig, ok := clusterConfig.NodePools[poolName]
	if !ok || poolConfig.Numa == nil {
		return nil
	}

	nrtObj, err := BuildNRT(nodeName, *poolConfig.Numa, len(nodeTopology.Gpus), node.Status.Allocatable)
	if err != nil {
		return fmt.Errorf("failed to build NodeResourceTopology for %s: %w", nodeName, err)
	}
	if nrtObj == nil {
		return nil
	}

	nrtObj.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "v1",
		Kind:       "Node",
		Name:       node.Name,
		UID:        node.UID,
	}}

	return publisher.Apply(ctx, nrtObj)
}
