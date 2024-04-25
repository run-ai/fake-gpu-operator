package app

type App interface {
	Run()
	GetConfig() interface{}
	Name() string
	Init(stop chan struct{})
}
