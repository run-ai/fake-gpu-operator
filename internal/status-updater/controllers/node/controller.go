package node

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers/util"
	"github.com/spf13/viper"

	nodehandler "github.com/run-ai/fake-gpu-operator/internal/status-updater/handlers/node"

	v1 "k8s.io/api/core/v1"
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
	err := c.pruneTopologyNodes()
	if err != nil {
		log.Fatalf("Failed to prune topology nodes: %v", err)
	}

	c.informer.Run(stopCh)
}

func (c *NodeController) pruneTopologyNodes() error {
	log.Print("Pruning topology nodes...")

	gpuNodes, err := c.kubeClient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{
		LabelSelector: "nvidia.com/gpu.deploy.dcgm-exporter=true,nvidia.com/gpu.deploy.device-plugin=true",
	})
	if err != nil {
		return fmt.Errorf("failed listing fake gpu nodes: %v", err)
	}

	nodeTopologyCms, err := c.kubeClient.CoreV1().ConfigMaps(viper.GetString(constants.EnvTopologyCmNamespace)).List(context.TODO(), metav1.ListOptions{
		LabelSelector: "node-topology=true",
	})
	if err != nil {
		return fmt.Errorf("failed listing fake gpu nodes: %v", err)
	}

	validNodeTopologyCMMap := make(map[string]bool)
	for _, node := range gpuNodes.Items {
		validNodeTopologyCMMap[topology.GetNodeTopologyCMName(node.Name)] = true
	}

	for _, cm := range nodeTopologyCms.Items {
		if _, ok := validNodeTopologyCMMap[cm.Name]; !ok {
			util.LogErrorIfExist(c.kubeClient.CoreV1().ConfigMaps(viper.GetString(constants.EnvTopologyCmNamespace)).Delete(context.TODO(), cm.Name, metav1.DeleteOptions{}), fmt.Sprintf("Failed to delete node topology cm %s", cm.Name))
		}
	}

	return nil
}

func isFakeGpuNode(node *v1.Node) bool {
	return node != nil &&
		node.Labels["nvidia.com/gpu.deploy.dcgm-exporter"] == "true" &&
		node.Labels["nvidia.com/gpu.deploy.device-plugin"] == "true"
}
