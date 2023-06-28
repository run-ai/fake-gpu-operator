package topology

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetNodeTopologyFromCM(kubeclient kubernetes.Interface, nodeName string) (*Node, error) {
	cmList, err := kubeclient.CoreV1().ConfigMaps(
		viper.GetString("TOPOLOGY_CM_NAMESPACE")).List(
		context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list configmaps: %v", err)
	}

	for _, cm := range cmList.Items {
		if cm.Name == getNodeTopologyCMName(nodeName) {
			return FromNodeConfigMap(&cm)
		}
	}

	return nil, fmt.Errorf("node topology configmap not found")
}

func CreateNodeTopologyCM(kubeclient kubernetes.Interface, nodeTopology *Node, nodeName string) error {
	cm, err := ToNodeTopologyConfigMap(nodeTopology, nodeName)
	if err != nil {
		return err
	}

	_, err = kubeclient.CoreV1().ConfigMaps(
		viper.GetString("TOPOLOGY_CM_NAMESPACE")).Create(context.TODO(), cm, metav1.CreateOptions{})
	return err
}

func UpdateNodeTopologyCM(kubeclient kubernetes.Interface, nodeTopology *Node, nodeName string) error {
	cm, err := ToNodeTopologyConfigMap(nodeTopology, nodeName)
	if err != nil {
		return err
	}

	_, err = kubeclient.CoreV1().ConfigMaps(
		viper.GetString("TOPOLOGY_CM_NAMESPACE")).Update(context.TODO(), cm, metav1.UpdateOptions{})
	return err
}

func DeleteNodeTopologyCM(kubeclient kubernetes.Interface, nodeName string) error {

	err := kubeclient.CoreV1().ConfigMaps(
		viper.GetString("TOPOLOGY_CM_NAMESPACE")).Delete(context.TODO(), getNodeTopologyCMName(nodeName), metav1.DeleteOptions{})
	return err
}

func GetFromKube(kubeclient kubernetes.Interface) (*Cluster, error) {
	topologyCm, err := kubeclient.CoreV1().ConfigMaps(
		viper.GetString("TOPOLOGY_CM_NAMESPACE")).Get(
		context.TODO(), viper.GetString("TOPOLOGY_CM_NAME"), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get topology configmap: %v", err)
	}

	cluster, err := FromConfigMap(topologyCm)
	if err != nil {
		return nil, fmt.Errorf("failed to parse topology configmap: %v", err)
	}

	return cluster, nil
}

func UpdateToKube(kubeclient kubernetes.Interface, clusterTopology *Cluster) error {
	topologyCm, err := ToConfigMap(clusterTopology)
	if err != nil {
		return err
	}

	_, err = kubeclient.CoreV1().ConfigMaps(
		viper.GetString("TOPOLOGY_CM_NAMESPACE")).Update(context.TODO(), topologyCm, metav1.UpdateOptions{})
	return err
}

func FromConfigMap(cm *corev1.ConfigMap) (*Cluster, error) {
	var clusterTopology Cluster
	err := json.Unmarshal([]byte(cm.Data[CmTopologyKey]), &clusterTopology)
	if err != nil {
		return nil, err
	}

	return &clusterTopology, nil
}

func FromNodeConfigMap(cm *corev1.ConfigMap) (*Node, error) {
	var nodeTopology Node
	err := json.Unmarshal([]byte(cm.Data[CmTopologyKey]), &nodeTopology)
	if err != nil {
		return nil, err
	}

	return &nodeTopology, nil
}

func ToConfigMap(clusterTopology *Cluster) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      viper.GetString("TOPOLOGY_CM_NAME"),
			Namespace: viper.GetString("TOPOLOGY_CM_NAMESPACE"),
		},
		Data: make(map[string]string),
	}

	topologyData, err := json.Marshal(clusterTopology)
	if err != nil {
		return nil, err
	}

	cm.Data[CmTopologyKey] = string(topologyData)

	return cm, nil
}

func ToNodeTopologyConfigMap(nodeTopology *Node, nodeName string) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getNodeTopologyCMName(nodeName),
			Namespace: viper.GetString("TOPOLOGY_CM_NAMESPACE"),
		},
		Data: make(map[string]string),
	}

	topologyData, err := json.Marshal(nodeTopology)
	if err != nil {
		return nil, err
	}

	cm.Data[CmTopologyKey] = string(topologyData)

	return cm, nil
}

func getNodeTopologyCMName(nodeName string) string {
	return viper.GetString("TOPOLOGY_CM_NAME") + "-" + nodeName
}
