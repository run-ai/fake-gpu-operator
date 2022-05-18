package topology

import (
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
)

func ParseConfigMap(cm *corev1.ConfigMap) (*ClusterTopology, error) {
	var clusterTopology ClusterTopology
	err := yaml.Unmarshal([]byte(cm.Data[CmTopologyKey]), &clusterTopology)
	if err != nil {
		return nil, err
	}

	return &clusterTopology, nil
}
