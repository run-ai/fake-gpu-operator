package migfaker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
)

var GenerateUuid = uuid.New

type MigFaker struct {
	kubeclient kubeclient.KubeClientInterface
}

func NewMigFaker(kubeclient kubeclient.KubeClientInterface) *MigFaker {
	return &MigFaker{
		kubeclient: kubeclient,
	}
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

		migDeviceMappingInfo, err := faker.getGpuMigDeviceMappingInfo(selectedDevice)
		if err != nil {
			return fmt.Errorf("failed to get gpu mig device mapping info: %w", err)
		}

		mappings[gpuIdx] = migDeviceMappingInfo
	}

	smappings, _ := json.Marshal(mappings)

	labels := map[string]string{
		constants.MigConfigStateLabel: "success",
	}
	annotations := map[string]string{
		constants.MigMappingAnnotation: base64.StdEncoding.EncodeToString(smappings),
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

func (faker *MigFaker) getGpuMigDeviceMappingInfo(devices SelectedDevices) ([]MigDeviceMappingInfo, error) {
	gpuProduct, err := faker.getGpuProduct()
	if err != nil {
		return nil, fmt.Errorf("failed to get gpu product: %w", err)
	}

	migDevices := []MigDeviceMappingInfo{}
	for _, migDevice := range devices.MigDevices {
		gpuInstanceId, err := migInstanceNameToGpuInstanceId(gpuProduct, migDevice.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get gpu instance id: %w", err)
		}
		migDevices = append(migDevices, MigDeviceMappingInfo{
			Position:      migDevice.Position,
			DeviceUUID:    fmt.Sprintf("MIG-%s", GenerateUuid()),
			GpuInstanceId: gpuInstanceId,
		})
	}

	return migDevices, nil
}

func (faker *MigFaker) getGpuProduct() (string, error) {
	nodeLabels, err := faker.kubeclient.GetNodeLabels()
	if err != nil {
		return "", fmt.Errorf("failed to get node labels: %w", err)
	}

	return nodeLabels[constants.GpuProductLabel], nil
}

func migInstanceNameToGpuInstanceId(gpuProduct string, migInstanceName string) (int, error) {
	var gpuInstanceId int
	var ok bool
	switch {
	case strings.Contains(gpuProduct, "40GB"):
		gpuInstanceId, ok = map[string]int{
			"1g.5gb":    19,
			"1g.5gb+me": 20,
			"1g.10gb":   15,
			"2g.10gb":   14,
			"3g.20gb":   9,
			"4g.20gb":   5,
			"7g.40gb":   0,
		}[migInstanceName]
	case strings.Contains(gpuProduct, "80GB"):
		gpuInstanceId, ok = map[string]int{
			"1g.10gb":    19,
			"1g.10gb+me": 20,
			"1g.20gb":    15,
			"2g.20gb":    14,
			"3g.40gb":    9,
			"4g.40gb":    5,
			"7g.80gb":    0,
		}[migInstanceName]
	default:
		return -1, fmt.Errorf("gpuProduct %s not supported", gpuProduct)
	}

	if !ok {
		return -1, fmt.Errorf("failed mapping mig instance name %s to gpu instance id", migInstanceName)
	}

	return gpuInstanceId, nil
}
