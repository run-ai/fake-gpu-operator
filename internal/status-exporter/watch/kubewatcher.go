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
	subscribers []chan<- *topology.NodeTopology
}

func NewKubeWatcher(kubeclient kubeclient.KubeClientInterface) *KubeWatcher {
	return &KubeWatcher{
		kubeclient: kubeclient,
	}
}

func (w *KubeWatcher) Subscribe(subscriber chan<- *topology.NodeTopology) {
	w.subscribers = append(w.subscribers, subscriber)
}

func (w *KubeWatcher) Watch(stopCh <-chan struct{}) {
	cmChan, err := w.kubeclient.WatchConfigMap(viper.GetString("TOPOLOGY_CM_NAMESPACE"), topology.GetNodeTopologyCMName(viper.GetString("NODE_NAME")))
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
			nodeTopology, err := topology.FromNodeTopologyCM(cm)
			if err != nil {
				panic(err)
			}
			w.publishTopology(nodeTopology)

		case <-ticker.C:
			log.Printf("Topology update not received within interval, publishing...\n")
			cm, ok := w.kubeclient.GetConfigMap(viper.GetString("TOPOLOGY_CM_NAMESPACE"), topology.GetNodeTopologyCMName(viper.GetString("NODE_NAME")))
			if !ok {
				break
			}
			nodeTopology, err := topology.FromNodeTopologyCM(cm)
			if err != nil {
				panic(err)
			}
			w.publishTopology(nodeTopology)

		case <-stopCh:
			return
		}
	}
}

func (w *KubeWatcher) publishTopology(nodeTopology *topology.NodeTopology) {
	for _, subscriber := range w.subscribers {
		subscriber <- nodeTopology
	}
}
