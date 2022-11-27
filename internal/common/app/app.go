package app

import "sync"

type App interface {
	Start()
	GetConfig() interface{}
	Name() string
	Init(stop chan struct{}, wg *sync.WaitGroup)
}
