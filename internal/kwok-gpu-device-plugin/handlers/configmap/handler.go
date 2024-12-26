package configmap

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

type Interface interface {
	HandleAdd(cm *v1.ConfigMap) error
}

type ConfigMapHandler struct {
	kubeClient kubernetes.Interface

	clusterTopology *topology.ClusterTopology
}

var _ Interface = &ConfigMapHandler{}

func NewConfigMapHandler(kubeClient kubernetes.Interface, clusterTopology *topology.ClusterTopology) *ConfigMapHandler {
	return &ConfigMapHandler{
		kubeClient:      kubeClient,
		clusterTopology: clusterTopology,
	}
}

func (p *ConfigMapHandler) HandleAdd(cm *v1.ConfigMap) error {
	log.Printf("Handling config map addition: %s\n", cm.Name)

	nodeTopology, err := topology.FromNodeTopologyCM(cm)
	if err != nil {
		return fmt.Errorf("failed to read node topology ConfigMap: %w", err)
	}
	nodeName := cm.Labels[constants.LabelTopologyCMNodeName]

	return p.applyFakeDevicePlugin(nodeTopology, nodeName)
}

func (p *ConfigMapHandler) applyFakeDevicePlugin(nodeTopology *topology.NodeTopology, nodeName string) error {
	nodePatch := &v1.Node{
		Status: v1.NodeStatus{
			Capacity: v1.ResourceList{
				v1.ResourceName(constants.GpuResourceName): *resource.NewQuantity(int64(len(nodeTopology.Gpus)), resource.DecimalSI),
			},
			Allocatable: v1.ResourceList{
				v1.ResourceName(constants.GpuResourceName): *resource.NewQuantity(int64(len(nodeTopology.Gpus)), resource.DecimalSI),
			},
		},
	}

	for _, otherDevice := range nodeTopology.OtherDevices {
		nodePatch.Status.Capacity[v1.ResourceName(otherDevice.Name)] = *resource.NewQuantity(int64(otherDevice.Count), resource.DecimalSI)
		nodePatch.Status.Allocatable[v1.ResourceName(otherDevice.Name)] = *resource.NewQuantity(int64(otherDevice.Count), resource.DecimalSI)
	}

	patchBytes, err := json.Marshal(nodePatch)
	if err != nil {
		return fmt.Errorf("failed to update node: failed to marshal patch: %v", err)
	}

	_, err = p.kubeClient.CoreV1().Nodes().Patch(
		context.TODO(), nodeName, types.MergePatchType, patchBytes, metav1.PatchOptions{}, "status",
	)
	if err != nil {
		return fmt.Errorf("failed to update node capacity and allocatable: %v", err)
	}

	return nil
}
