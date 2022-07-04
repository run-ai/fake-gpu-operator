package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	conf, err := rest.InClusterConfig()
	if err != nil {
		panic(err)
	}

	kubeclient := kubernetes.NewForConfigOrDie(conf)

	http.HandleFunc("/topology", func(w http.ResponseWriter, r *http.Request) {
		clusterTopology, err := topology.GetFromKube(kubeclient)
		if err != nil {
			panic(err)
		}

		clusterTopologyJSON, err := json.Marshal(clusterTopology)
		if err != nil {
			panic(err)
		}

		log.Printf("Returning cluster topology: %s", clusterTopologyJSON)

		w.Header().Set("Content-Type", "application/json")
		w.Write(clusterTopologyJSON)
	})

	log.Printf("Serving on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
