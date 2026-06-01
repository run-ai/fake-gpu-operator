package migfaker_test

import (
	"context"
	"fmt"
	"testing"

	"encoding/base64"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/run-ai/fake-gpu-operator/internal/migfaker"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestFakeMapping(t *testing.T) {
	viper.Set(constants.EnvNodeName, "node1")
	viper.Set(constants.EnvTopologyCmName, "topology")
	viper.Set(constants.EnvTopologyCmNamespace, "gpu-operator")
	oldGenerateUUID := migfaker.GenerateUuid
	t.Cleanup(func() {
		migfaker.GenerateUuid = oldGenerateUUID
		viper.Reset()
	})

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

	nodeTopology := &topology.NodeTopology{
		GpuProduct:  "NVIDIA-A100-SXM4-40GB",
		MigStrategy: "mixed",
		Gpus: []topology.GpuDetails{
			{ID: "GPU-0"},
		},
	}
	topologyCM, _, err := topology.ToNodeTopologyCM(nodeTopology, "node1")
	assert.NoError(t, err)

	devicePluginPod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "device-plugin-abc",
			Namespace: "gpu-operator",
			Labels: map[string]string{
				"app":       "device-plugin",
				"component": "device-plugin",
			},
		},
		Spec: v1.PodSpec{NodeName: "node1"},
	}
	clientset := fake.NewSimpleClientset(topologyCM, devicePluginPod)

	kubeClientMock := &kubeclient.KubeClientMock{}
	kubeClientMock.ActualSetNodeLabels = func(labels map[string]string) {
		assert.Equal(t, labels["nvidia.com/mig.config.state"], "success")
	}
	kubeClientMock.ActualGetNodeLabels = func() (map[string]string, error) {
		return map[string]string{
			constants.LabelGpuProduct: "NVIDIA-A100-SXM4-40GB",
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

	migFaker := migfaker.NewMigFaker(kubeClientMock, clientset)
	err = migFaker.FakeMapping(migConfig)
	if err != nil {
		t.Errorf("Failed to fake mapping %s", err)
	}

	updatedCM, err := clientset.CoreV1().ConfigMaps("gpu-operator").Get(context.TODO(), topology.GetNodeTopologyCMName("node1"), metav1.GetOptions{})
	assert.NoError(t, err)
	updatedTopology, err := topology.FromNodeTopologyCM(updatedCM)
	assert.NoError(t, err)
	assert.True(t, updatedTopology.Gpus[0].MigEnabled)
	assert.Len(t, updatedTopology.Gpus[0].MigInstances, 1)
	assert.Equal(t, "4g.20gb", updatedTopology.Gpus[0].MigInstances[0].Profile)
	assert.Equal(t, 0, updatedTopology.Gpus[0].MigInstances[0].Index)
	assert.Equal(t, fmt.Sprintf("MIG-%s", uid), updatedTopology.Gpus[0].MigInstances[0].UUID)

	_, err = clientset.CoreV1().Pods("gpu-operator").Get(context.TODO(), "device-plugin-abc", metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
}

func TestFakeMappingAllDevicesCreatesUniqueMigInstancesPerGpu(t *testing.T) {
	viper.Set(constants.EnvNodeName, "node1")
	viper.Set(constants.EnvTopologyCmName, "topology")
	viper.Set(constants.EnvTopologyCmNamespace, "gpu-operator")
	uuids := []uuid.UUID{uuid.New(), uuid.New()}
	idx := 0
	oldGenerateUUID := migfaker.GenerateUuid
	t.Cleanup(func() {
		migfaker.GenerateUuid = oldGenerateUUID
		viper.Reset()
	})
	migfaker.GenerateUuid = func() uuid.UUID {
		u := uuids[idx]
		idx++
		return u
	}

	nodeTopology := &topology.NodeTopology{
		GpuProduct:  "NVIDIA-A100-SXM4-40GB",
		MigStrategy: "mixed",
		Gpus: []topology.GpuDetails{
			{ID: "GPU-0"},
			{ID: "GPU-1"},
		},
	}
	topologyCM, _, err := topology.ToNodeTopologyCM(nodeTopology, "node1")
	assert.NoError(t, err)
	clientset := fake.NewSimpleClientset(topologyCM)

	kubeClientMock := &kubeclient.KubeClientMock{
		ActualSetNodeLabels: func(labels map[string]string) {
			assert.Equal(t, "success", labels[constants.LabelMigConfigState])
		},
		ActualSetNodeAnnotations: func(annotations map[string]string) {
			assert.NotEmpty(t, annotations[constants.AnnotationMigMapping])
		},
	}

	migFaker := migfaker.NewMigFaker(kubeClientMock, clientset)
	err = migFaker.FakeMapping(&migfaker.MigConfigs{
		SelectedDevices: []migfaker.SelectedDevices{
			{
				Devices:    []string{"all"},
				MigEnabled: true,
				MigDevices: []migfaker.MigDevice{
					{Name: "1g.5gb", Position: 0, Size: 1},
				},
			},
		},
	})
	assert.NoError(t, err)

	updatedCM, err := clientset.CoreV1().ConfigMaps("gpu-operator").Get(context.TODO(), topology.GetNodeTopologyCMName("node1"), metav1.GetOptions{})
	assert.NoError(t, err)
	updatedTopology, err := topology.FromNodeTopologyCM(updatedCM)
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("MIG-%s", uuids[0]), updatedTopology.Gpus[0].MigInstances[0].UUID)
	assert.Equal(t, fmt.Sprintf("MIG-%s", uuids[1]), updatedTopology.Gpus[1].MigInstances[0].UUID)
}
