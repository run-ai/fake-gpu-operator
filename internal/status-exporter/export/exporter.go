package export

type Interface interface {
	Run(stopCh <-chan struct{})
}
