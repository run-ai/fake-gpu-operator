package app

import "sync"

type App interface {
	Run()
	GetConfig() interface{}
	Name() string
	Init(stop chan struct{}, wg *sync.WaitGroup)
}
