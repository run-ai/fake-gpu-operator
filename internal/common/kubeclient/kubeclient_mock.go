package kubeclient

import (
	corev1 "k8s.io/api/core/v1"
)

type KubeClientMock struct {
	ActualSetNodeLabels      func(labels map[string]string)
	ActualSetNodeAnnotations func(annotations map[string]string)
	ActualWatchConfigMap     func(namespace string, configmapName string)
}

// SetNodeAnnotations implements kubeclient.KubeClientInterface
func (client *KubeClientMock) SetNodeAnnotations(annotations map[string]string) error {
	client.ActualSetNodeAnnotations(annotations)
	return nil
}

func (client *KubeClientMock) SetNodeLabels(labels map[string]string) error {
	client.ActualSetNodeLabels(labels)
	return nil
}

func (client *KubeClientMock) WatchConfigMap(namespace string, configmapName string) (chan *corev1.ConfigMap, error) {
	client.ActualWatchConfigMap(namespace, configmapName)
	return nil, nil
}

func (client *KubeClientMock) GetConfigMap(namespace string, configmapName string) (*corev1.ConfigMap, bool) {
	return nil, true
}
