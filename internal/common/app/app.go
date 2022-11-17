package app

import "sync"

type App interface {
	Start(stopper chan struct{}, wg *sync.WaitGroup)
	Name() string
}
