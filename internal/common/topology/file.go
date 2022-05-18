package topology

import (
	"os"

	"gopkg.in/yaml.v2"
)

func GetClusterTopologyFromFs(topologyPath string) (*ClusterTopology, error) {
	// Open file
	file, err := os.Open(topologyPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Decode yaml file
	var clusterTopology ClusterTopology
	err = yaml.NewDecoder(file).Decode(&clusterTopology)
	if err != nil {
		return nil, err
	}

	return &clusterTopology, nil
}

func GetNodeTopologyFromFs(topologyPath string, nodeName string) (*NodeTopology, error) {
	clusterTopology, err := GetClusterTopologyFromFs(topologyPath)
	if err != nil {
		return nil, err
	}

	return clusterTopology.GetNodeTopology(nodeName)
}
