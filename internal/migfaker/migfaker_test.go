package migfaker_test

import (
	"testing"

	"encoding/base64"

	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/run-ai/fake-gpu-operator/internal/migfaker"
	"github.com/stretchr/testify/assert"
)

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
	kubeClientMock := &kubeclient.KubeClientMock{}
	kubeClientMock.ActualSetNodeLabels = func(labels map[string]string) {
		assert.Equal(t, labels["nvidia.com/mig.config.state"], "true")
	}

	kubeClientMock.ActualSetNodeAnnotations = func(labels map[string]string) {
		b64mapping := labels["run.ai/mig-mapping"]
		mapping, _ := base64.StdEncoding.DecodeString(b64mapping)
		assert.JSONEq(t, string(mapping), `{"0":{"4":"2g.10gb"}}`)
	}

	migFaker := migfaker.NewMigFaker(kubeClientMock)
	migFaker.FakeMapping(migConfig)
}
