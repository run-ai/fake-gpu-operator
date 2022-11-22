package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

func main() {
	kubeclient := kubeclient.NewKubeClient(nil, nil)

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

	log.Printf("Serving on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
