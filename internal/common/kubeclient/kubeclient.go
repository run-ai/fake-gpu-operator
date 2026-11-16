package kubeclient

import (
	"context"
	"log"
	"time"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	ctrl "sigs.k8s.io/controller-runtime"
)

type KubeClientInterface interface {
	SetNodeLabels(lables map[string]string) error
	SetNodeAnnotations(annotations map[string]string) error
	GetNodeLabels() (map[string]string, error)
	WatchConfigMap(namespace string, configmapName string) (chan *corev1.ConfigMap, error)
	GetConfigMap(namespace string, configmapName string) (*corev1.ConfigMap, bool)
}

type KubeClient struct {
	ClientSet kubernetes.Interface
	stopChan  chan struct{}
}

func NewKubeClient(config *rest.Config, stop chan struct{}) *KubeClient {
	if config == nil {
		var err error
		config, err = ctrl.GetConfig()
		if err != nil {
			log.Fatalf("Error getting in cluster config to init kubeclient: %e", err)
		}
	}

	clientset := kubernetes.NewForConfigOrDie(config)
	return &KubeClient{
		ClientSet: clientset,
		stopChan:  stop,
	}
}

func (client *KubeClient) SetNodeLabels(lables map[string]string) error {
	nodeName := viper.GetString(constants.EnvNodeName)
	node, err := client.ClientSet.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	for k, v := range lables {
		node.Labels[k] = v
	}

	log.Printf("Labelling node %s with %v\n", nodeName, lables)
	_, err = client.ClientSet.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
	return err
}

func (client *KubeClient) SetNodeAnnotations(annotations map[string]string) error {
	nodeName := viper.GetString(constants.EnvNodeName)
	node, err := client.ClientSet.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	for k, v := range annotations {
		node.Annotations[k] = v
	}

	log.Printf("Labelling node %s with %v\n", nodeName, annotations)
	_, err = client.ClientSet.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
	return err
}

func (client *KubeClient) GetNodeLabels() (map[string]string, error) {
	nodeName := viper.GetString(constants.EnvNodeName)
	node, err := client.ClientSet.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return node.Labels, nil
}

func (client *KubeClient) GetConfigMap(namespace string, configmapName string) (*corev1.ConfigMap, bool) {
	cm, err := client.ClientSet.CoreV1().ConfigMaps(
		namespace).Get(
		context.TODO(), configmapName, metav1.GetOptions{})
	if err != nil {
		log.Printf("Error getting configmap: %s", configmapName)
		return cm, false
	}
	return cm, true
}

// watchRetryInterval is the wait between attempts to re-open a failed watch.
var watchRetryInterval = 5 * time.Second

func (client *KubeClient) WatchConfigMap(namespace string, configmapName string) (chan *corev1.ConfigMap, error) {
	cmWatch, err := client.watchConfigMap(namespace, configmapName)
	if err != nil {
		log.Printf("Error watching configmap: %s", configmapName)
		return nil, err
	}

	configMapsChan := make(chan *corev1.ConfigMap)
	go client.watchCmChange(cmWatch, namespace, configmapName, configMapsChan)

	return configMapsChan, nil
}

func (client *KubeClient) watchConfigMap(namespace string, configmapName string) (watch.Interface, error) {
	return client.ClientSet.CoreV1().ConfigMaps(
		namespace).Watch(context.TODO(), metav1.ListOptions{
		FieldSelector: "metadata.name=" + configmapName,
		Watch:         true,
	})
}

// watchCmChange forwards ConfigMap updates until stopChan closes. The apiserver
// ends every watch after a timeout, closing ResultChan, so it re-opens the watch.
func (client *KubeClient) watchCmChange(cmWatch watch.Interface, namespace string, configmapName string, configMapsChan chan *corev1.ConfigMap) {
	for {
		select {
		case result, ok := <-cmWatch.ResultChan():
			if !ok {
				if cmWatch = client.rewatchConfigMap(namespace, configmapName); cmWatch == nil {
					return
				}
				continue
			}
			if result.Type == watch.Added || result.Type == watch.Modified {
				if cm, ok := result.Object.(*corev1.ConfigMap); ok {
					select {
					case configMapsChan <- cm:
					case <-client.stopChan:
						cmWatch.Stop()
						return
					}
				}
			}
		case <-client.stopChan:
			cmWatch.Stop()
			return
		}
	}
}

// rewatchConfigMap retries until the watch opens. It returns nil if stopChan
// closes first.
func (client *KubeClient) rewatchConfigMap(namespace string, configmapName string) watch.Interface {
	for {
		cmWatch, err := client.watchConfigMap(namespace, configmapName)
		if err == nil {
			return cmWatch
		}
		log.Printf("Error re-watching configmap %s: %v", configmapName, err)
		select {
		case <-time.After(watchRetryInterval):
		case <-client.stopChan:
			return nil
		}
	}
}
