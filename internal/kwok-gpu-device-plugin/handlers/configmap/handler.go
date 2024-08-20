package configmap

import (
	"context"
	"fmt"
	"log"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	v1 "k8s.io/api/core/v1"
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

	return p.applyFakeDevicePlugin(len(nodeTopology.Gpus), nodeName)
}

func (p *ConfigMapHandler) applyFakeDevicePlugin(gpuCount int, nodeName string) error {
	patch := fmt.Sprintf(
		`{"status": {"capacity": {"%s": "%d"}, "allocatable": {"%s": "%d"}}}`,
		constants.GpuResourceName, gpuCount, constants.GpuResourceName, gpuCount,
	)
	_, err := p.kubeClient.CoreV1().Nodes().Patch(
		context.TODO(), nodeName, types.MergePatchType, []byte(patch), metav1.PatchOptions{}, "status",
	)
	if err != nil {
		return fmt.Errorf("failed to update node capacity and allocatable: %v", err)
	}

	return nil
}
