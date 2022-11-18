package app

import "sync"

type App interface {
	Start(stopper chan struct{}, wg *sync.WaitGroup)
	GetConfig() interface{}
	Name() string
}
