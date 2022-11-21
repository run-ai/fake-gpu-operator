package kubeclient

type KubeClientMock struct {
	ActualSetNodeLabels      func(labels map[string]string)
	ActualSetNodeAnnotations func(annotations map[string]string)
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
