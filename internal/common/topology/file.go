package topology

import (
	"encoding/json"
	"os"
)

func GetClusterTopologyFromFs(topologyPath string) (*Cluster, error) {
	// Open file
	file, err := os.Open(topologyPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Decode json file
	var clusterTopology Cluster
	err = json.NewDecoder(file).Decode(&clusterTopology)
	if err != nil {
		return nil, err
	}

	return &clusterTopology, nil
}

func GetNodeTopologyFromFs(topologyPath string, nodeName string) (*Node, error) {
	clusterTopology, err := GetClusterTopologyFromFs(topologyPath)
	if err != nil {
		return nil, err
	}

	return clusterTopology.GetNodeTopology(nodeName)
}
