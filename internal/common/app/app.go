package app

import "sync"

type App interface {
	Start(wg *sync.WaitGroup)
	GetConfig() interface{}
	Name() string
	Init(stop chan struct{})
}
