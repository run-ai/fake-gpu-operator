package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/api/errors"
)

func main() {
	kubeclient := kubeclient.NewKubeClient(nil, nil)
	viper.AutomaticEnv()
	http.HandleFunc("/topology", func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Content-Type", "application/json")
		baseTopology, err := topology.GetBaseTopologyFromCM(kubeclient.ClientSet)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		baseTopologyJSON, err := json.Marshal(baseTopology)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		log.Printf("Returning cluster topology: %s", baseTopologyJSON)

		_, err = w.Write(baseTopologyJSON)
		if err != nil {
			panic(err)
		}
	})

	http.HandleFunc("/topology/nodes/", func(w http.ResponseWriter, r *http.Request) {
		nodeName := strings.Split(r.URL.Path, "/")[3]
		w.Header().Set("Content-Type", "application/json")

		if nodeName == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Can't get node name from url " + r.URL.Path))
			return
		}

		nodeTopology, err := topology.GetNodeTopologyFromCM(kubeclient.ClientSet, nodeName)
		if err != nil {
			if errors.IsNotFound(err) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("Node topology not found"))
			} else {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(err.Error()))
			}
			return
		}

		nodeTopologyJSON, err := json.Marshal(nodeTopology)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		log.Printf("Returning node topology: %s", nodeTopologyJSON)

		_, err = w.Write(nodeTopologyJSON)
		if err != nil {
			panic(err)
		}
	})

	log.Printf("Serving on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
