package topology

import (
	"context"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/spf13/viper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetNodeTopologyFromCM(kubeclient kubernetes.Interface, nodeName string) (*NodeTopology, error) {
	cmName := GetNodeTopologyCMName(nodeName)
	cm, err := kubeclient.CoreV1().ConfigMaps(
		viper.GetString(constants.EnvTopologyCmNamespace)).Get(
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
		viper.GetString(constants.EnvTopologyCmNamespace)).Create(context.TODO(), cm, metav1.CreateOptions{})
	return err
}

func UpdateNodeTopologyCM(kubeclient kubernetes.Interface, nodeTopology *NodeTopology, nodeName string) error {
	cm, err := ToNodeTopologyCM(nodeTopology, nodeName)
	if err != nil {
		return err
	}

	_, err = kubeclient.CoreV1().ConfigMaps(
		viper.GetString(constants.EnvTopologyCmNamespace)).Update(context.TODO(), cm, metav1.UpdateOptions{})
	return err
}

func DeleteNodeTopologyCM(kubeclient kubernetes.Interface, nodeName string) error {
	err := kubeclient.CoreV1().ConfigMaps(
		viper.GetString(constants.EnvTopologyCmNamespace)).Delete(context.TODO(), GetNodeTopologyCMName(nodeName), metav1.DeleteOptions{})
	return err
}

func GetClusterTopologyFromCM(kubeclient kubernetes.Interface) (*ClusterTopology, error) {
	topologyCm, err := kubeclient.CoreV1().ConfigMaps(
		viper.GetString(constants.EnvTopologyCmNamespace)).Get(
		context.TODO(), viper.GetString(constants.EnvTopologyCmName), metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get topology configmap: %v", err)
	}

	cluster, err := FromClusterTopologyCM(topologyCm)
	if err != nil {
		return nil, fmt.Errorf("failed to parse topology configmap: %v", err)
	}

	return cluster, nil
}

func FromClusterTopologyCM(cm *corev1.ConfigMap) (*ClusterTopology, error) {
	var clusterTopology ClusterTopology
	err := yaml.Unmarshal([]byte(cm.Data[cmTopologyKey]), &clusterTopology)
	if err != nil {
		return nil, err
	}

	return &clusterTopology, nil
}

func FromNodeTopologyCM(cm *corev1.ConfigMap) (*NodeTopology, error) {
	var nodeTopology NodeTopology
	err := yaml.Unmarshal([]byte(cm.Data[cmTopologyKey]), &nodeTopology)
	if err != nil {
		return nil, err
	}

	return &nodeTopology, nil
}

func ToClusterTopologyCM(clusterTopology *ClusterTopology) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      viper.GetString(constants.EnvTopologyCmName),
			Namespace: viper.GetString(constants.EnvTopologyCmNamespace),
		},
		Data: make(map[string]string),
	}

	topologyData, err := yaml.Marshal(clusterTopology)
	if err != nil {
		return nil, err
	}

	cm.Data[cmTopologyKey] = string(topologyData)

	return cm, nil
}

func ToNodeTopologyCM(nodeTopology *NodeTopology, nodeName string) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetNodeTopologyCMName(nodeName),
			Namespace: viper.GetString(constants.EnvTopologyCmNamespace),
			Labels: map[string]string{
				constants.LabelTopologyCMNodeTopology: "true",
				constants.LabelTopologyCMNodeName:     nodeName,
			},
		},
		Data: make(map[string]string),
	}

	topologyData, err := yaml.Marshal(nodeTopology)
	if err != nil {
		return nil, err
	}

	cm.Data[cmTopologyKey] = string(topologyData)

	return cm, nil
}

func GetNodeTopologyCMName(nodeName string) string {
	return viper.GetString(constants.EnvTopologyCmName) + "-" + nodeName
}
