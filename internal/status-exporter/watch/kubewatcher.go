package watch

import (
	"context"
	"log"
	"os"

	"github.com/run-ai/gpu-mock-stack/internal/common/topology"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// type kubewatcher
type KubeWatcher struct {
	kubeclient  kubernetes.Interface
	subscribers []chan<- *topology.ClusterTopology
}

func NewKubeWatcher(kubeclient kubernetes.Interface) *KubeWatcher {
	return &KubeWatcher{
		kubeclient: kubeclient,
	}
}

func (w *KubeWatcher) Subscribe(subscriber chan<- *topology.ClusterTopology) {
	w.subscribers = append(w.subscribers, subscriber)
}

func (w *KubeWatcher) Watch(stopCh <-chan struct{}) {
	cmWatch, err := w.kubeclient.CoreV1().ConfigMaps(os.Getenv("TOPOLOGY_CM_NAMESPACE")).Watch(context.TODO(), metav1.ListOptions{
		FieldSelector: "metadata.name=" + os.Getenv("TOPOLOGY_CM_NAME"),
		Watch:         true,
	})
	if err != nil {
		panic(err)
	}

	for {
		select {
		case e := <-cmWatch.ResultChan():
			if e.Type == "ADDED" || e.Type == "MODIFIED" {
				if cm, ok := e.Object.(*corev1.ConfigMap); ok {
					log.Printf("Got topology update, publishing...\n")
					w.publishTopology(cm)
				}
			}
		case <-stopCh:
			return
		}
	}
}

func (w *KubeWatcher) publishTopology(cm *corev1.ConfigMap) {
	clusterTopology, err := topology.ParseConfigMap(cm)
	if err != nil {
		panic(err)
	}

	for _, subscriber := range w.subscribers {
		subscriber <- clusterTopology
	}
}
