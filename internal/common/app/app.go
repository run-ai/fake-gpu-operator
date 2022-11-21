package app

import "sync"

type App interface {
	Start(stop chan struct{}, wg *sync.WaitGroup)
	GetConfig() interface{}
	Name() string
	Init()
}
