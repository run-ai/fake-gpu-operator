/*
Subscribes to pod updates and handles them.
When a pod is added, we'll mark a random GPU on the pod's node as fully utilized
When a pod is removed, we'll unmark the GPU.
*/
package pod

import (
	"log"
	"os"
	"sync"

	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers"
	podhandler "github.com/run-ai/fake-gpu-operator/internal/status-updater/handlers/pod"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/util"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type PodEventHandler struct {
	kubeClient kubernetes.Interface
	wg         *sync.WaitGroup

	informer cache.SharedIndexInformer
	handler  podhandler.Interface
}

var _ controllers.Interface = &PodEventHandler{}

func NewPodController(kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, wg *sync.WaitGroup) *PodEventHandler {
	p := &PodEventHandler{
		kubeClient: kubeClient,
		wg:         wg,
		informer:   informers.NewSharedInformerFactory(kubeClient, 0).Core().V1().Pods().Informer(),
		handler:    podhandler.NewPodHandler(kubeClient, dynamicClient),
	}

	p.informer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			switch pod := obj.(type) {
			case *v1.Pod:
				return (pod != nil) &&
					(util.IsDedicatedGpuPod(pod) || util.IsSharedGpuPod(pod))
			default:
				return false
			}
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				if isPodRunning(pod) {
					handleError(p.handler.HandleAdd(pod), "add")
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				oldPod := oldObj.(*v1.Pod)
				newPod := newObj.(*v1.Pod)

				isOldPodRunning := isPodRunning(oldPod)
				isNewPodRunning := isPodRunning(newPod)

				if isOldPodRunning == isNewPodRunning {
					return
				}

				if isNewPodRunning {
					handleError(p.handler.HandleAdd(newPod), "add")
				} else {
					handleError(p.handler.HandleDelete(newPod), "delete")
				}
			},
			DeleteFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				if isPodRunning(pod) {
					handleError(p.handler.HandleDelete(pod), "delete")
				}
			},
		},
	})

	// GuyTodo: Remove after RUN-6464 is resolved
	err := p.resetTopologyStatus()
	if err != nil {
		log.Printf("Error resetting topology status: %v\n", err)
		log.Printf("Please reset the topology status manually\n")
		log.Printf("Exiting...\n")
		os.Exit(1)
	}

	return p
}

func (p *PodEventHandler) Run(stopCh <-chan struct{}) {
	defer p.wg.Done()

	p.informer.Run(stopCh)
}

func handleError(err error, operation string) {
	if err != nil {
		log.Printf("Error handling pod %s: %v\n", operation, err)
	}
}
