/*
Watches for changes in the topology status and sends the new topology to all subscribers.
*/
package watch

import (
	"github.com/run-ai/gpu-mock-stack/internal/common/topology"
)

type Interface interface {
	Subscribe(subscriber chan<- *topology.ClusterTopology)
	Watch(stopCh <-chan struct{})
}
