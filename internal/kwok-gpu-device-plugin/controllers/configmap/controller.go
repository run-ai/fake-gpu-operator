package configmamp

import (
	"log"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers/util"

	cmhandler "github.com/run-ai/fake-gpu-operator/internal/kwok-gpu-device-plugin/handlers/configmap"

	v1 "k8s.io/api/core/v1"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type ConfigMapController struct {
	kubeClient kubernetes.Interface
	cmInformer cache.SharedIndexInformer
	handler    cmhandler.Interface

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
					util.LogErrorIfExist(c.handler.HandleAdd(obj.(*v1.ConfigMap)), "Failed to handle cm addition")
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
	c.cmInformer.Run(stopCh)
}

func (c *ConfigMapController) isFakeGpuKWOKNodeConfigMap(cm *v1.ConfigMap) bool {
	if cm == nil || cm.Labels == nil || cm.Annotations == nil {
		return false
	}
	_, foundNodeName := cm.Labels[constants.LabelTopologyCMNodeName]
	if !foundNodeName {
		return false
	}

	return cm.Annotations[constants.AnnotationKwokNode] == "fake"
}
