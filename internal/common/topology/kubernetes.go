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

	return ParseConfigMap(topologyCm)
}

func ParseConfigMap(cm *corev1.ConfigMap) (*ClusterTopology, error) {
	var clusterTopology ClusterTopology
	err := yaml.Unmarshal([]byte(cm.Data[CmTopologyKey]), &clusterTopology)
	if err != nil {
		return nil, err
	}

	return &clusterTopology, nil
}
