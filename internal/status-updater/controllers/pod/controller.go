package pod

import (
	"log"
	"sync"

	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers"
	controllers_util "github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers/util"
	podhandler "github.com/run-ai/fake-gpu-operator/internal/status-updater/handlers/pod"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/util"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type PodController struct {
	kubeClient kubernetes.Interface
	wg         *sync.WaitGroup

	informer cache.SharedIndexInformer
	handler  podhandler.Interface
}

var _ controllers.Interface = &PodController{}

func NewPodController(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, wg *sync.WaitGroup) *PodController {
	c := &PodController{
		kubeClient: kubeClient,
		wg:         wg,
		informer:   informers.NewSharedInformerFactory(kubeClient, 0).Core().V1().Pods().Informer(),
		handler:    podhandler.NewPodHandler(kubeClient, dynamicClient),
	}

	podAddFailureMsg := "Failed to handle pod addition"
	podDeleteFailureMsg := "Failed to handle pod deletion"

	_, err := c.informer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			switch pod := obj.(type) {
			case *v1.Pod:
				return (pod != nil) &&
					util.IsPodRunning(pod) &&
					(util.IsDedicatedGpuPod(pod) || util.IsSharedGpuPod(pod))
			default:
				return false
			}
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				controllers_util.LogErrorIfExist(c.handler.HandleAdd(pod), podAddFailureMsg)
			},
			DeleteFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				controllers_util.LogErrorIfExist(c.handler.HandleDelete(pod), podDeleteFailureMsg)
			},
		},
	})
	if err != nil {
		log.Fatalf("Failed to add pod event handler: %v", err)
	}

	return c
}

func (p *PodController) Run(stopCh <-chan struct{}) {
	defer p.wg.Done()

	p.informer.Run(stopCh)
}
