package migfaker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"

	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
)

var fakeLables = map[string]string{
	"node-role.kubernetes.io/runai-dynamic-mig":   "true",
	"node-role.kubernetes.io/runai-mig-enabled":   "true",
	"feature.node.kubernetes.io/pci-10de.present": "true",
}

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

func (faker *MigFaker) FakeMapping(config *MigConfigs) {
	mappings := map[string]map[string]string{}
	for id, selectedDevice := range config.SelectedDevices {
		mappings[fmt.Sprint(id)] = faker.copyMigDevices(selectedDevice)
	}

	smappings, _ := json.Marshal(mappings)

	labels := map[string]string{
		"nvidia.com/mig.config.state": "true",
	}
	annotations := map[string]string{
		"run.ai/mig-mapping": base64.StdEncoding.EncodeToString(smappings),
	}

	err := faker.kubeclient.SetNodeLabels(labels)
	if err != nil {
		log.Printf("error on setting node labels: %e", err)
	}
	err = faker.kubeclient.SetNodeAnnotations(annotations)
	if err != nil {
		log.Printf("error on setting node annotations: %e", err)
	}
}

func (*MigFaker) copyMigDevices(device SelectedDevices) map[string]string {
	migDevices := map[string]string{}
	for key, val := range device.MigDevices {
		migDevices[key] = val
	}
	return migDevices
}
