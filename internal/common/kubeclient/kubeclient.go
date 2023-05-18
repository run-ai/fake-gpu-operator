package kubeclient

import (
	"context"
	"log"

	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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
		config, err = rest.InClusterConfig()
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
	nodeName := viper.GetString("NODE_NAME")
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
	nodeName := viper.GetString("NODE_NAME")
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
	nodeName := viper.GetString("NODE_NAME")
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

func (client *KubeClient) WatchConfigMap(namespace string, configmapName string) (chan *corev1.ConfigMap, error) {
	cmWatch, err := client.ClientSet.CoreV1().ConfigMaps(
		namespace).Watch(context.TODO(), metav1.ListOptions{
		FieldSelector: "metadata.name=" + configmapName,
		Watch:         true,
	})
	if err != nil {
		log.Printf("Error watching configmap: %s", configmapName)
		return nil, err
	}

	configMapsChan := make(chan *corev1.ConfigMap)
	go client.watchCmChange(cmWatch, configMapsChan)

	return configMapsChan, nil
}

func (client *KubeClient) watchCmChange(cmWatch watch.Interface, configMapsChan chan *corev1.ConfigMap) {
	for {
		select {
		case result := <-cmWatch.ResultChan():
			if result.Type == "ADDED" || result.Type == "MODIFIED" {
				if cm, ok := result.Object.(*corev1.ConfigMap); ok {
					configMapsChan <- cm
				}
			}
		case <-client.stopChan:
		}
	}
}
