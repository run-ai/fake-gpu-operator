package migfaker

import (
	"sync"

	"github.com/spf13/viper"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	ResourceNodes = "nodes"
)

type SyncableMigConfig struct {
	cond     *sync.Cond
	mutex    sync.Mutex
	current  string
	lastRead string
}

func NewSyncableMigConfig() *SyncableMigConfig {
	var m SyncableMigConfig
	m.cond = sync.NewCond(&m.mutex)
	return &m
}

func (m *SyncableMigConfig) Set(value string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.current = value
	if m.current != "" {
		m.cond.Broadcast()
	}
}

func (m *SyncableMigConfig) Get() string {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.lastRead == m.current {
		m.cond.Wait()
	}
	m.lastRead = m.current
	return m.lastRead
}

func ContinuouslySyncMigConfigChanges(clientset kubernetes.Interface, migConfig *SyncableMigConfig, stop chan struct{}) {
	listWatch := cache.NewListWatchFromClient(
		clientset.CoreV1().RESTClient(),
		ResourceNodes,
		v1.NamespaceAll,
		fields.OneTermEqualSelector("metadata.name", viper.GetString("NODE_NAME")),
	)

	_, controller := cache.NewInformer(
		listWatch, &v1.Node{}, 0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				migConfig.Set(obj.(*v1.Node).Annotations[MigConfigAnnotation])
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				oldAnnotation := oldObj.(*v1.Node).Annotations[MigConfigAnnotation]
				newAnnotation := newObj.(*v1.Node).Annotations[MigConfigAnnotation]
				if oldAnnotation != newAnnotation {
					migConfig.Set(newAnnotation)
				}
			},
		},
	)

	go controller.Run(stop)
}
