package topology

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetNodeTopologyFromCM(kubeclient kubernetes.Interface, nodeName string) (*NodeTopology, error) {
	cmName := GetNodeTopologyCMName(nodeName)
	cm, err := kubeclient.CoreV1().ConfigMaps(
		viper.GetString("TOPOLOGY_CM_NAMESPACE")).Get(
		context.TODO(), cmName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return FromNodeTopologyCM(cm)
}

func CreateNodeTopologyCM(kubeclient kubernetes.Interface, nodeTopology *NodeTopology, nodeName string) error {
	cm, err := ToNodeTopologyCM(nodeTopology, nodeName)
	if err != nil {
		return err
	}

	_, err = kubeclient.CoreV1().ConfigMaps(
		viper.GetString("TOPOLOGY_CM_NAMESPACE")).Create(context.TODO(), cm, metav1.CreateOptions{})
	return err
}

func UpdateNodeTopologyCM(kubeclient kubernetes.Interface, nodeTopology *NodeTopology, nodeName string) error {
	cm, err := ToNodeTopologyCM(nodeTopology, nodeName)
	if err != nil {
		return err
	}

	_, err = kubeclient.CoreV1().ConfigMaps(
		viper.GetString("TOPOLOGY_CM_NAMESPACE")).Update(context.TODO(), cm, metav1.UpdateOptions{})
	return err
}

func DeleteNodeTopologyCM(kubeclient kubernetes.Interface, nodeName string) error {

	err := kubeclient.CoreV1().ConfigMaps(
		viper.GetString("TOPOLOGY_CM_NAMESPACE")).Delete(context.TODO(), GetNodeTopologyCMName(nodeName), metav1.DeleteOptions{})
	return err
}

func GetBaseTopologyFromCM(kubeclient kubernetes.Interface) (*BaseTopology, error) {
	topologyCm, err := kubeclient.CoreV1().ConfigMaps(
		viper.GetString("TOPOLOGY_CM_NAMESPACE")).Get(
		context.TODO(), viper.GetString("TOPOLOGY_CM_NAME"), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get topology configmap: %v", err)
	}

	cluster, err := FromBaseTopologyCM(topologyCm)
	if err != nil {
		return nil, fmt.Errorf("failed to parse topology configmap: %v", err)
	}

	return cluster, nil
}

func FromBaseTopologyCM(cm *corev1.ConfigMap) (*BaseTopology, error) {
	var baseTopology BaseTopology
	err := yaml.Unmarshal([]byte(cm.Data[CmTopologyKey]), &baseTopology)
	if err != nil {
		return nil, err
	}

	return &baseTopology, nil
}

func FromNodeTopologyCM(cm *corev1.ConfigMap) (*NodeTopology, error) {
	var nodeTopology NodeTopology
	err := yaml.Unmarshal([]byte(cm.Data[CmTopologyKey]), &nodeTopology)
	if err != nil {
		return nil, err
	}

	return &nodeTopology, nil
}

func ToBaseTopologyCM(baseTopology *BaseTopology) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      viper.GetString("TOPOLOGY_CM_NAME"),
			Namespace: viper.GetString("TOPOLOGY_CM_NAMESPACE"),
		},
		Data: make(map[string]string),
	}

	topologyData, err := yaml.Marshal(baseTopology)
	if err != nil {
		return nil, err
	}

	cm.Data[CmTopologyKey] = string(topologyData)

	return cm, nil
}

func ToNodeTopologyCM(nodeTopology *NodeTopology, nodeName string) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetNodeTopologyCMName(nodeName),
			Namespace: viper.GetString("TOPOLOGY_CM_NAMESPACE"),
			Labels: map[string]string{
				"node-topology": "true",
			},
		},
		Data: make(map[string]string),
	}

	topologyData, err := yaml.Marshal(nodeTopology)
	if err != nil {
		return nil, err
	}

	cm.Data[CmTopologyKey] = string(topologyData)

	return cm, nil
}

func GetNodeTopologyCMName(nodeName string) string {
	return viper.GetString("TOPOLOGY_CM_NAME") + "-" + nodeName
}
