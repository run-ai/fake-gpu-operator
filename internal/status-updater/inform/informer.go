/* Informs of relevant pod changes */
package inform

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type Interface interface {
	Subscribe(chan<- *PodEvent)
	Run(stopCh <-chan struct{})
}

type Informer struct {
	kubeclient  kubernetes.Interface
	subscribers []chan<- *PodEvent
	podInformer cache.SharedIndexInformer
}

var _ Interface = &Informer{}

func NewInformer(kubeclient kubernetes.Interface) *Informer {
	w := &Informer{
		kubeclient:  kubeclient,
		subscribers: make([]chan<- *PodEvent, 0),
		podInformer: informers.NewSharedInformerFactory(kubeclient, 0).Core().V1().Pods().Informer(),
	}

	w.podInformer.AddEventHandler(cache.FilteringResourceEventHandler{
		FilterFunc: func(obj interface{}) bool {
			switch pod := obj.(type) {
			case *v1.Pod:
				return isPodRequestingGpu(pod)
			default:
				return false
			}
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				if isPodRunning(pod) {
					w.publishPodEvent(pod, ADD)
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

				eventType := ADD
				if !isNewPodRunning {
					eventType = DELETE
				}

				w.publishPodEvent(newPod, eventType)
			},
			DeleteFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				if isPodRunning(pod) {
					w.publishPodEvent(pod, DELETE)
				}
			},
		},
	})

	return w
}

func (inf *Informer) Subscribe(subscriber chan<- *PodEvent) {
	inf.subscribers = append(inf.subscribers, subscriber)
}

func (inf *Informer) Run(stopCh <-chan struct{}) {
	defer inf.closeSubscribers()

	inf.podInformer.Run(stopCh)
}

func (inf *Informer) closeSubscribers() {
	for _, subscriber := range inf.subscribers {
		close(subscriber)
	}
}

func (inf *Informer) publishPodEvent(pod *v1.Pod, eventType EventType) {
	for _, subscriber := range inf.subscribers {
		subscriber <- &PodEvent{
			Pod:       pod,
			EventType: eventType,
		}
	}
}

func isPodRunning(pod *v1.Pod) bool {
	return pod.Status.Phase == v1.PodRunning
}

func isPodRequestingGpu(pod *v1.Pod) bool {
	return !pod.Spec.Containers[0].Resources.Limits["nvidia.com/gpu"].Equal(resource.MustParse("0"))
}
