package node

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/hashicorp/go-multierror"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers/util"
	"github.com/spf13/viper"

	nodehandler "github.com/run-ai/fake-gpu-operator/internal/status-updater/handlers/node"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type NodeController struct {
	kubeClient kubernetes.Interface
	informer   cache.SharedIndexInformer
	handler    nodehandler.Interface
}

var _ controllers.Interface = &NodeController{}

func NewNodeController(kubeClient kubernetes.Interface, wg *sync.WaitGroup) *NodeController {
	c := &NodeController{
		kubeClient: kubeClient,
		informer:   informers.NewSharedInformerFactory(kubeClient, 0).Core().V1().Nodes().Informer(),
		handler:    nodehandler.NewNodeHandler(kubeClient),
	}

	_, err := c.informer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			switch node := obj.(type) {
			case *v1.Node:
				return isFakeGpuNode(node)
			default:
				return false
			}
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				node := obj.(*v1.Node)
				util.LogErrorIfExist(c.handler.HandleAdd(node), "Failed to handle node addition")
			},
			DeleteFunc: func(obj interface{}) {
				node := obj.(*v1.Node)
				util.LogErrorIfExist(c.handler.HandleDelete(node), "Failed to handle node deletion")
			},
		},
	})
	if err != nil {
		log.Fatalf("Failed to add node event handler: %v", err)
	}

	return c
}

func (c *NodeController) Run(stopCh <-chan struct{}) {
	err := c.pruneTopologyConfigMaps()
	if err != nil {
		log.Fatalf("Failed to prune topology nodes: %v", err)
	}

	c.informer.Run(stopCh)
}

// This function prunes the topology ConfigMaps that are not associated with any fake gpu nodes, and initializes the GpuTopologyStatus field in the remaining ConfigMaps.
func (c *NodeController) pruneTopologyConfigMaps() error {
	log.Print("Pruning topology ConfigMaps...")

	gpuNodes, err := c.kubeClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{
		LabelSelector: "nvidia.com/gpu.deploy.dcgm-exporter=true,nvidia.com/gpu.deploy.device-plugin=true",
	})
	if err != nil {
		return fmt.Errorf("failed listing fake gpu nodes: %v", err)
	}

	nodeTopologyCms, err := c.kubeClient.CoreV1().ConfigMaps(viper.GetString(constants.EnvTopologyCmNamespace)).List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=true", constants.LabelTopologyCMNodeTopology),
	})
	if err != nil {
		return fmt.Errorf("failed listing fake gpu nodes: %v", err)
	}

	validNodeTopologyCMMap := make(map[string]bool)
	for _, node := range gpuNodes.Items {
		validNodeTopologyCMMap[topology.GetNodeTopologyCMName(node.Name)] = true
	}

	var multiErr error
	for _, cm := range nodeTopologyCms.Items {
		_, ok := validNodeTopologyCMMap[cm.Name]
		multiErr = multierror.Append(multiErr, c.pruneTopologyConfigMap(&cm, ok))
	}

	return nil
}

func (c *NodeController) pruneTopologyConfigMap(cm *v1.ConfigMap, isValidNodeTopologyCM bool) error {
	if !isValidNodeTopologyCM {
		util.LogErrorIfExist(c.kubeClient.CoreV1().ConfigMaps(viper.GetString(constants.EnvTopologyCmNamespace)).Delete(context.TODO(), cm.Name, metav1.DeleteOptions{}), fmt.Sprintf("Failed to delete node topology cm %s", cm.Name))
	}

	nodeTopology, err := topology.FromNodeTopologyCM(cm)
	if err != nil {
		return fmt.Errorf("failed to parse node topology cm %s: %v", cm.Name, err)
	}

	for i := range nodeTopology.Gpus {
		nodeTopology.Gpus[i].Status.PodGpuUsageStatus = topology.PodGpuUsageStatusMap{}

		// Remove non-existing pods from the allocation info
		allocatingPodExists, err := isPodExist(c.kubeClient, nodeTopology.Gpus[i].Status.AllocatedBy.Pod, nodeTopology.Gpus[i].Status.AllocatedBy.Namespace)
		if err != nil {
			return fmt.Errorf("failed to check if pod %s exists: %v", nodeTopology.Gpus[i].Status.AllocatedBy.Pod, err)
		}

		if !allocatingPodExists {
			nodeTopology.Gpus[i].Status.AllocatedBy = topology.ContainerDetails{}
		}
	}

	nodeName, ok := cm.ObjectMeta.Labels[constants.LabelTopologyCMNodeName]
	if !ok {
		return fmt.Errorf("node topology cm %s does not have node name label", cm.Name)
	}

	err = topology.UpdateNodeTopologyCM(c.kubeClient, nodeTopology, nodeName)
	if err != nil {
		return fmt.Errorf("failed to update node topology cm %s: %v", cm.Name, err)
	}

	return nil
}

func isFakeGpuNode(node *v1.Node) bool {
	return node != nil &&
		node.Labels["nvidia.com/gpu.deploy.dcgm-exporter"] == "true" &&
		node.Labels["nvidia.com/gpu.deploy.device-plugin"] == "true"
}

func isPodExist(kubeClient kubernetes.Interface, podName string, namespace string) (bool, error) {
	_, err := kubeClient.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return false, err
		}
		return false, nil
	}
	return true, nil
}
