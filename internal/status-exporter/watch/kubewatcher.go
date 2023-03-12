package watch

import (
	"log"
	"time"

	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/spf13/viper"
)

const defaultMaxExportInterval = 10 * time.Second

// type kubewatcher
type KubeWatcher struct {
	kubeclient  kubeclient.KubeClientInterface
	subscribers []chan<- *topology.Cluster
}

func NewKubeWatcher(kubeclient kubeclient.KubeClientInterface) *KubeWatcher {
	return &KubeWatcher{
		kubeclient: kubeclient,
	}
}

func (w *KubeWatcher) Subscribe(subscriber chan<- *topology.Cluster) {
	w.subscribers = append(w.subscribers, subscriber)
}

func (w *KubeWatcher) Watch(stopCh <-chan struct{}) {
	cmChan, err := w.kubeclient.WatchConfigMap(viper.GetString("TOPOLOGY_CM_NAMESPACE"), viper.GetString("TOPOLOGY_CM_NAME"))
	if err != nil {
		panic(err)
	}

	viper.SetDefault("TOPOLOGY_MAX_EXPORT_INTERVAL", defaultMaxExportInterval)
	maxInterval := viper.GetDuration("TOPOLOGY_MAX_EXPORT_INTERVAL")
	ticker := time.NewTicker(maxInterval)

	for {
		select {
		case cm := <-cmChan:
			ticker.Reset(maxInterval)
			log.Printf("Got topology update, publishing...\n")
			clusterTopology, err := topology.FromConfigMap(cm)
			if err != nil {
				panic(err)
			}
			w.publishTopology(clusterTopology)

		case <-ticker.C:
			log.Printf("Topology update not received within interval, publishing...\n")
			cm, ok := w.kubeclient.GetConfigMap(viper.GetString("TOPOLOGY_CM_NAMESPACE"), viper.GetString("TOPOLOGY_CM_NAME"))
			if !ok {
				break
			}
			clusterTopology, err := topology.FromConfigMap(cm)
			if err != nil {
				panic(err)
			}
			w.publishTopology(clusterTopology)

		case <-stopCh:
			return
		}
	}
}

func (w *KubeWatcher) publishTopology(clusterTopology *topology.Cluster) {
	for _, subscriber := range w.subscribers {
		subscriber <- clusterTopology
	}
}
