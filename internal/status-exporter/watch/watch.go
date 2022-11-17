/*
Watches for changes in the topology status and sends the new topology to all subscribers.
*/
package watch

import (
	"sync"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

type Interface interface {
	Subscribe(subscriber chan<- *topology.ClusterTopology)
	Watch(stopCh <-chan struct{}, wg *sync.WaitGroup)
}
