package migfaker_test

import (
	"fmt"
	"testing"

	"encoding/base64"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
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
				MigDevices: []migfaker.MigDevice{
					{
						Name:     "4g.20gb",
						Position: 0,
						Size:     4,
					},
				},
			},
		},
	}
	migfaker.GenerateUuid = func() uuid.UUID { return uid }
	kubeClientMock := &kubeclient.KubeClientMock{}
	kubeClientMock.ActualSetNodeLabels = func(labels map[string]string) {
		assert.Equal(t, labels["nvidia.com/mig.config.state"], "success")
	}
	kubeClientMock.ActualGetNodeLabels = func() (map[string]string, error) {
		return map[string]string{
			constants.GpuProductLabel: "NVIDIA-A100-SXM4-40GB",
		}, nil
	}

	kubeClientMock.ActualSetNodeAnnotations = func(labels map[string]string) {
		b64mapping := labels["run.ai/mig-mapping"]
		actualMappingJson, _ := base64.StdEncoding.DecodeString(b64mapping)

		expectedMapping := migfaker.MigMapping{
			0: []migfaker.MigDeviceMappingInfo{
				{
					Position:      0,
					DeviceUUID:    fmt.Sprintf("MIG-%s", uid),
					GpuInstanceId: 5,
				},
			},
		}
		expectedMappingJson, err := json.Marshal(expectedMapping)

		assert.NoError(t, err)
		assert.JSONEq(t, string(expectedMappingJson), string(actualMappingJson))
	}

	migFaker := migfaker.NewMigFaker(kubeClientMock)
	err := migFaker.FakeMapping(migConfig)
	if err != nil {
		t.Errorf("Failed to fake mapping %s", err)
	}
}
