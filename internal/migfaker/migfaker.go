package migfaker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strconv"

	"github.com/google/uuid"
	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
)

var fakeLables = map[string]string{
	"feature.node.kubernetes.io/pci-10de.present": "true",
	"node-role.kubernetes.io/runai-dynamic-mig":   "true",
	"node-role.kubernetes.io/runai-mig-enabled":   "true",
}

var GenerateUuid = uuid.New

type MigFaker struct {
	kubeclient kubeclient.KubeClientInterface
}

func NewMigFaker(kubeclient kubeclient.KubeClientInterface) *MigFaker {
	return &MigFaker{
		kubeclient: kubeclient,
	}
}

func (faker *MigFaker) FakeNodeLabels() error {
	return faker.kubeclient.SetNodeLabels(fakeLables)
}

func (faker *MigFaker) FakeMapping(config *MigConfigs) error {
	mappings := MigMapping{}
	for _, selectedDevice := range config.SelectedDevices {
		if len(selectedDevice.Devices) == 0 {
			continue
		}

		gpuIdx, err := strconv.Atoi(selectedDevice.Devices[0])
		if err != nil {
			return fmt.Errorf("failed to parse gpu index %s: %w", selectedDevice.Devices[0], err)
		}
		mappings[gpuIdx] = faker.copyMigDevices(selectedDevice)
	}

	smappings, _ := json.Marshal(mappings)

	labels := map[string]string{
		"nvidia.com/mig.config.state": "success",
	}
	annotations := map[string]string{
		"run.ai/mig-mapping": base64.StdEncoding.EncodeToString(smappings),
	}

	err := faker.kubeclient.SetNodeLabels(labels)
	if err != nil {
		log.Printf("error on setting node labels: %e", err)
		return err
	}
	err = faker.kubeclient.SetNodeAnnotations(annotations)
	if err != nil {
		log.Printf("error on setting node annotations: %e", err)
		return err
	}
	return nil
}

func (*MigFaker) copyMigDevices(devices SelectedDevices) []MigDeviceMappingInfo {
	migDevices := []MigDeviceMappingInfo{}
	for _, migDevice := range devices.MigDevices {
		migDevices = append(migDevices, MigDeviceMappingInfo{
			Position:      migDevice.Position,
			DeviceUUID:    fmt.Sprintf("MIG-%s", GenerateUuid()),
			GpuInstanceId: 0,
		})
	}
	return migDevices
}
