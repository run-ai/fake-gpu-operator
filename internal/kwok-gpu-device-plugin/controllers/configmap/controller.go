package configmamp

import (
	"context"
	"log"
	"time"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers/util"

	cmhandler "github.com/run-ai/fake-gpu-operator/internal/kwok-gpu-device-plugin/handlers/configmap"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	maxRetryCount  = 10
	baseRetryDelay = time.Millisecond * 100
)

type ConfigMapController struct {
	kubeClient      kubernetes.Interface
	cmInformer      cache.SharedIndexInformer
	nodeLister      listersv1.NodeLister
	informerFactory informers.SharedInformerFactory
	handler         cmhandler.Interface

	clusterTopology *topology.ClusterTopology
}

var _ controllers.Interface = &ConfigMapController{}

func NewConfigMapController(
	kubeClient kubernetes.Interface, namespace string,
) *ConfigMapController {
	clusterTopology, err := topology.GetClusterTopologyFromCM(kubeClient)
	if err != nil {
		log.Fatalf("Failed to get cluster topology: %v", err)
	}

	informerFactory := informers.NewSharedInformerFactoryWithOptions(
		kubeClient, 0, informers.WithNamespace(namespace))
	c := &ConfigMapController{
		kubeClient:      kubeClient,
		cmInformer:      informerFactory.Core().V1().ConfigMaps().Informer(),
		nodeLister:      informerFactory.Core().V1().Nodes().Lister(),
		informerFactory: informerFactory,
		handler:         cmhandler.NewConfigMapHandler(kubeClient, clusterTopology),
		clusterTopology: clusterTopology,
	}

	_, err = c.cmInformer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			switch cm := obj.(type) {
			case *v1.ConfigMap:
				return c.isFakeGpuKWOKNodeConfigMap(cm)
			default:
				return false
			}
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				go func() {
					c.callConfigMapHandler(obj.(*v1.ConfigMap), 0)
				}()
			},
		},
	})
	if err != nil {
		log.Fatalf("Failed to add config map event handler: %v", err)
	}

	return c
}

func (c *ConfigMapController) Run(stopCh <-chan struct{}) {
	log.Println("Starting config map controller")
	c.informerFactory.Start(stopCh)
}

func (c *ConfigMapController) isFakeGpuKWOKNodeConfigMap(cm *v1.ConfigMap) bool {
	if cm == nil || cm.Labels == nil {
		return false
	}
	nodeName, foundNodeName := cm.Labels[constants.LabelTopologyCMNodeName]
	if !foundNodeName {
		return false
	}

	node, err := c.nodeLister.Get(nodeName)
	if err != nil {
		node, err = c.kubeClient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
		if err != nil {
			log.Printf("Failed to get node %s: %v", nodeName, err)
			return false
		}
	}

	return node.Annotations[constants.AnnotationKwokNode] == "fake"
}

func (c *ConfigMapController) callConfigMapHandler(cm *v1.ConfigMap, retryCount int) {
	nodeName := cm.Labels[constants.LabelTopologyCMNodeName]
	node, err := c.nodeLister.Get(nodeName)
	if err != nil {
		delay := baseRetryDelay * (1 << retryCount)
		log.Printf("Failed to get node %s: %v. retry in %v", nodeName, err, delay)
		time.Sleep(delay)
		if retryCount < maxRetryCount {
			c.callConfigMapHandler(cm, retryCount+1)
		}
		return
	}
	util.LogErrorIfExist(c.handler.HandleAdd(cm, node), "Failed to handle cm addition")
}
