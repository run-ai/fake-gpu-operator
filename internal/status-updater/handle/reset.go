package handle

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (p *PodEventHandler) resetTopologyStatus() error {
	log.Println("Resetting topology status")
	cm, clusterTopology, err := p.getTopology()
	if err != nil {
		return err
	}

	if clusterTopology.Config.NodeAutofill.Enabled && len(clusterTopology.Nodes) == 0 {
		log.Println("Auto configuring nodes")
		clusterTopology.Config.NodeAutofill.Enabled = false
		if err = p.autoConfigNodes(clusterTopology); err != nil {
			log.Println("Error auto configuring nodes:", err)
		}
	}

	for nodeName, node := range clusterTopology.Nodes {
		if node.GpuCount != 0 && len(node.Gpus) != node.GpuCount {
			log.Printf("Node %s has %d GPUs, but %d GPUs are requested. Generating GPUs...\n", nodeName, len(node.Gpus), node.GpuCount)
			node.Gpus = generateGpuDetails(node.GpuCount, nodeName)
			clusterTopology.Nodes[nodeName] = node
		}

		for gpuIdx := range node.Gpus {
			clusterTopology.Nodes[nodeName].Gpus[gpuIdx].Status = topology.GpuStatus{}
		}
	}

	return p.updateTopology(clusterTopology, cm)
}

func (p *PodEventHandler) autoConfigNodes(clusterTopology *topology.ClusterTopology) error {
	if err := p.validateAutoConfigNodesSettings(&clusterTopology.Config.NodeAutofill); err != nil {
		return err
	}

	nodes, err := p.kubeclient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("error getting nodes: %v", err)
	}

	relevantNodes := []string{}
	for _, node := range nodes.Items {
		if node.Labels["nvidia.com/gpu.deploy.dcgm-exporter"] == "true" && node.Labels["nvidia.com/gpu.deploy.device-plugin"] == "true" {
			relevantNodes = append(relevantNodes, node.Name)
		}
	}

	if clusterTopology.Nodes == nil {
		clusterTopology.Nodes = make(map[string]topology.NodeTopology)
	}

	for _, nodeName := range relevantNodes {
		clusterTopology.Nodes[nodeName] = topology.NodeTopology{
			GpuCount:   clusterTopology.Config.NodeAutofill.GpuCount,
			GpuMemory:  clusterTopology.Config.NodeAutofill.GpuMemory,
			GpuProduct: clusterTopology.Config.NodeAutofill.GpuProduct,
		}
	}

	return nil
}

func (p *PodEventHandler) validateAutoConfigNodesSettings(settings *topology.NodeAutofillSettings) error {
	if settings.GpuCount == 0 {
		return fmt.Errorf("gpu-count must be greater than 0")
	}

	if settings.GpuMemory == 0 {
		return fmt.Errorf("gpu-memory must be greater than 0")
	}

	if settings.GpuProduct == "" {
		return fmt.Errorf("gpu-product must be set")
	}

	return nil
}

func generateGpuDetails(gpuCount int, nodeName string) []topology.GpuDetails {
	gpus := make([]topology.GpuDetails, gpuCount)
	for idx := range gpus {
		gpus[idx] = topology.GpuDetails{
			ID: fmt.Sprintf("GPU-%s", uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("%s-%d", nodeName, idx)))),
		}
	}

	return gpus
}
