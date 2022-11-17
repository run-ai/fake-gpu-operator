package export

import "sync"

type Interface interface {
	Run(stopCh <-chan struct{}, wg *sync.WaitGroup)
}
