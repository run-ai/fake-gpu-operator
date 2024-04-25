package controllers

type Interface interface {
	Run(stopCh <-chan struct{})
}
