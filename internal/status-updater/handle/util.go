package handle

import (
	"context"
	"fmt"
	"log"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (p *PodEventHandler) getTopology() (*v1.ConfigMap, *topology.ClusterTopology, error) {
	cm, err := p.kubeclient.CoreV1().ConfigMaps(
		viper.GetString("TOPOLOGY_CM_NAMESPACE")).Get(
		context.TODO(), viper.GetString("TOPOLOGY_CM_NAME"), metav1.GetOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("error getting topology configmap: %v", err)
	}

	var clusterTopology topology.ClusterTopology
	err = yaml.Unmarshal([]byte(cm.Data[topology.CmTopologyKey]), &clusterTopology)
	if err != nil {
		return nil, nil, fmt.Errorf("error unmarshalling topology configmap: %v", err)
	}

	return cm, &clusterTopology, nil
}

func (p *PodEventHandler) updateTopology(newTopology *topology.ClusterTopology, cm *v1.ConfigMap) error {
	newTopologyYaml, err := yaml.Marshal(newTopology)
	if err != nil {
		return err
	}

	cm.Data[topology.CmTopologyKey] = string(newTopologyYaml)

	_, err = p.kubeclient.CoreV1().ConfigMaps(
		viper.GetString("TOPOLOGY_CM_NAMESPACE")).Update(context.TODO(), cm, metav1.UpdateOptions{})
	if err != nil {
		log.Printf("error updating topology configmap: %s\n", err)
	}
	return err
}
