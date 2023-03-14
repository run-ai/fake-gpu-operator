package migfaker_test

import (
	"fmt"
	"testing"

	"encoding/base64"

	"github.com/google/uuid"
	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/run-ai/fake-gpu-operator/internal/migfaker"
	"github.com/stretchr/testify/assert"
)

func TestFakeMapping(t *testing.T) {
	uid := uuid.New()
	migConfig := &migfaker.MigConfigs{
		SelectedDevices: []migfaker.SelectedDevices{
			{
				Devices:    []string{"0"},
				MigEnabled: true,
				MigDevices: map[string]string{
					"4": uid.String(),
				},
			},
		},
	}
	migfaker.GenerateUuid = func() uuid.UUID { return uid }
	kubeClientMock := &kubeclient.KubeClientMock{}
	kubeClientMock.ActualSetNodeLabels = func(labels map[string]string) {
		assert.Equal(t, labels["nvidia.com/mig.config.state"], "success")
	}

	kubeClientMock.ActualSetNodeAnnotations = func(labels map[string]string) {
		b64mapping := labels["run.ai/mig-mapping"]
		mapping, _ := base64.StdEncoding.DecodeString(b64mapping)
		assert.JSONEq(t, string(mapping), fmt.Sprintf(`{"0":{"4":"MIG-%s"}}`, uid))
	}

	migFaker := migfaker.NewMigFaker(kubeClientMock)
	err := migFaker.FakeMapping(migConfig)
	if err != nil {
		t.Errorf("Failed to fake mapping %s", err)
	}
}
