package watch

import (
	"log"
	"sync"
	"time"

	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/spf13/viper"
)

const defaultMaxExportInterval = 10 * time.Second

// type kubewatcher
type KubeWatcher struct {
	kubeclient  kubeclient.KubeClientInterface
	subscribers []chan<- *topology.ClusterTopology
	wg          *sync.WaitGroup
}

func NewKubeWatcher(kubeclient kubeclient.KubeClientInterface, wg *sync.WaitGroup) *KubeWatcher {
	return &KubeWatcher{
		kubeclient: kubeclient,
		wg:         wg,
	}
}

func (w *KubeWatcher) Subscribe(subscriber chan<- *topology.ClusterTopology) {
	w.subscribers = append(w.subscribers, subscriber)
}

func (w *KubeWatcher) Watch(stopCh <-chan struct{}) {
	defer w.wg.Done()
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

func (w *KubeWatcher) publishTopology(clusterTopology *topology.ClusterTopology) {
	for _, subscriber := range w.subscribers {
		subscriber <- clusterTopology
	}
}
