package migfaker_test

import (
	"testing"

	"encoding/base64"

	"github.com/run-ai/fake-gpu-operator/internal/migfaker"
	"github.com/stretchr/testify/assert"
)

var actualSetNodeLabels func(labels map[string]string)
var actualSetNodeAnnotations func(annotations map[string]string)

type FakeKubeClient struct {
}

// SetNodeAnnotations implements kubeclient.KubeClientInterface
func (*FakeKubeClient) SetNodeAnnotations(annotations map[string]string) error {
	actualSetNodeAnnotations(annotations)
	return nil
}

func (client *FakeKubeClient) SetNodeLabels(labels map[string]string) error {
	actualSetNodeLabels(labels)
	return nil
}

func TestFakeMapping(t *testing.T) {
	migConfig := &migfaker.MigConfigs{
		SelectedDevices: []migfaker.SelectedDevices{
			{
				Devices:    []string{"0"},
				MigEnabled: true,
				MigDevices: map[string]string{
					"4": "2g.10gb",
				},
			},
		},
	}

	actualSetNodeLabels = func(labels map[string]string) {
		assert.Equal(t, labels["nvidia.com/mig.config.state"], "true")
	}

	actualSetNodeAnnotations = func(labels map[string]string) {
		b64mapping := labels["run.ai/mig-mapping"]
		mapping, _ := base64.StdEncoding.DecodeString(b64mapping)
		assert.JSONEq(t, string(mapping), `{"0":{"4":"2g.10gb"}}`)
	}

	migFaker := migfaker.NewMigFaker(&FakeKubeClient{})
	migFaker.FakeMapping(migConfig)
}
