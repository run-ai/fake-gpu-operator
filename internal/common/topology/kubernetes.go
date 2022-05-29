package topology

import (
	"context"
	"os"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetFromKube(kubeclient kubernetes.Interface) (*ClusterTopology, error) {
	topologyCm, err := kubeclient.CoreV1().ConfigMaps(os.Getenv("TOPOLOGY_CM_NAMESPACE")).Get(context.TODO(), os.Getenv("TOPOLOGY_CM_NAME"), metav1.GetOptions{})
	if err != nil {
		panic(err)
	}

	return FromConfigMap(topologyCm)
}

func FromConfigMap(cm *corev1.ConfigMap) (*ClusterTopology, error) {
	var clusterTopology ClusterTopology
	err := yaml.Unmarshal([]byte(cm.Data[CmTopologyKey]), &clusterTopology)
	if err != nil {
		return nil, err
	}

	return &clusterTopology, nil
}

func ToConfigMap(clusterTopology *ClusterTopology) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      os.Getenv("TOPOLOGY_CM_NAME"),
			Namespace: os.Getenv("TOPOLOGY_CM_NAMESPACE"),
		},
		Data: make(map[string]string),
	}

	topologyData, err := yaml.Marshal(clusterTopology)
	if err != nil {
		return nil, err
	}

	cm.Data[CmTopologyKey] = string(topologyData)

	return cm, nil
}
