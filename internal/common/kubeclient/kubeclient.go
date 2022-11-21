package kubeclient

import (
	"context"
	"log"

	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type KubeClientInterface interface {
	SetNodeLabels(lables map[string]string) error
	SetNodeAnnotations(annotations map[string]string) error
}

type KubeClient struct {
	ClientSet kubernetes.Interface
}

func NewKubeClient(client kubernetes.Interface) *KubeClient {
	return &KubeClient{
		ClientSet: client,
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

	log.Printf("labelling node %s with %v\n", nodeName, lables)
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

	log.Printf("labelling node %s with %v\n", nodeName, annotations)
	_, err = client.ClientSet.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
	return err
}
