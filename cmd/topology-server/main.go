package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/spf13/viper"
)

func main() {
	kubeclient := kubeclient.NewKubeClient(nil, nil)
	viper.SetDefault("TOPOLOGY_CM_NAME", os.Getenv("TOPOLOGY_CM_NAME"))
	viper.SetDefault("TOPOLOGY_CM_NAMESPACE", os.Getenv("TOPOLOGY_CM_NAMESPACE"))
	http.HandleFunc("/topology", func(w http.ResponseWriter, r *http.Request) {
		cm, ok := kubeclient.GetConfigMap(os.Getenv("TOPOLOGY_CM_NAMESPACE"), os.Getenv("TOPOLOGY_CM_NAME"))
		if !ok {
			panic("Can't get topology")
		}
		clusterTopology, err := topology.FromConfigMap(cm)
		if err != nil {
			panic(err)
		}

		clusterTopologyJSON, err := json.Marshal(clusterTopology)
		if err != nil {
			panic(err)
		}

		log.Printf("Returning cluster topology: %s", clusterTopologyJSON)

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(clusterTopologyJSON)
		if err != nil {
			panic(err)
		}
	})

	http.HandleFunc("/topology/nodes/", func(w http.ResponseWriter, r *http.Request) {
		nodeName := strings.Split(r.URL.Path, "/")[3]
		if nodeName == "" {
			panic("Can't get node name from url " + r.URL.Path)
		}
		cm, ok := kubeclient.GetConfigMap(os.Getenv("TOPOLOGY_CM_NAMESPACE"), topology.GetNodeTopologyCMName(nodeName))
		if !ok {
			panic("Can't get node topology for node " + nodeName)
		}
		nodeTopology, err := topology.FromNodeConfigMap(cm)
		if err != nil {
			panic(err)
		}

		nodeTopologyJSON, err := json.Marshal(nodeTopology)
		if err != nil {
			panic(err)
		}

		log.Printf("Returning node topology: %s", nodeTopologyJSON)

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(nodeTopologyJSON)
		if err != nil {
			panic(err)
		}
	})

	log.Printf("Serving on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
