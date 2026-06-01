package migfaker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

var GenerateUuid = uuid.New

type MigFaker struct {
	kubeclient kubeclient.KubeClientInterface
	clientset  kubernetes.Interface
}

func NewMigFaker(kubeclient kubeclient.KubeClientInterface, clientset kubernetes.Interface) *MigFaker {
	return &MigFaker{
		kubeclient: kubeclient,
		clientset:  clientset,
	}
}

func (faker *MigFaker) FakeMapping(config *MigConfigs) error {
	nodeTopology, err := faker.getNodeTopology()
	if err != nil {
		return err
	}

	mappings, migInstances, err := faker.buildMigState(config, nodeTopology)
	if err != nil {
		return err
	}

	smappings, err := json.Marshal(mappings)
	if err != nil {
		return fmt.Errorf("failed to marshal MIG mapping: %w", err)
	}

	labels := map[string]string{
		constants.LabelMigConfigState: "success",
	}
	annotations := map[string]string{
		constants.AnnotationMigMapping: base64.StdEncoding.EncodeToString(smappings),
	}

	err = faker.kubeclient.SetNodeLabels(labels)
	if err != nil {
		log.Printf("error on setting node labels: %e", err)
		return err
	}
	err = faker.kubeclient.SetNodeAnnotations(annotations)
	if err != nil {
		log.Printf("error on setting node annotations: %e", err)
		return err
	}

	if err := faker.updateNodeTopology(nodeTopology, migInstances); err != nil {
		return err
	}

	if err := faker.restartDevicePluginPod(); err != nil {
		return err
	}

	return nil
}

func (faker *MigFaker) buildMigState(config *MigConfigs, nodeTopology *topology.NodeTopology) (MigMapping, map[int][]topology.MigInstance, error) {
	mappings := MigMapping{}
	migInstances := map[int][]topology.MigInstance{}
	gpuProduct := nodeTopology.GpuProduct
	if gpuProduct == "" {
		var err error
		gpuProduct, err = faker.getGpuProduct()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get gpu product: %w", err)
		}
	}

	for _, selectedDevice := range config.SelectedDevices {
		if len(selectedDevice.Devices) == 0 {
			continue
		}

		gpuIndices, err := expandGPUIndices(selectedDevice.Devices, len(nodeTopology.Gpus))
		if err != nil {
			return nil, nil, err
		}

		for _, gpuIdx := range gpuIndices {
			mappingInfo, topologyInstances, err := buildGpuMigDeviceState(gpuProduct, selectedDevice)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get gpu mig device mapping info: %w", err)
			}
			mappings[gpuIdx] = append(mappings[gpuIdx], mappingInfo...)
			migInstances[gpuIdx] = append(migInstances[gpuIdx], topologyInstances...)
		}
	}

	return mappings, migInstances, nil
}

func buildGpuMigDeviceState(gpuProduct string, devices SelectedDevices) ([]MigDeviceMappingInfo, []topology.MigInstance, error) {
	if !devices.MigEnabled {
		return nil, nil, nil
	}

	mappingInfo := []MigDeviceMappingInfo{}
	topologyInstances := []topology.MigInstance{}
	for _, migDevice := range devices.MigDevices {
		gpuInstanceId, err := migInstanceNameToGpuInstanceId(gpuProduct, migDevice.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get gpu instance id: %w", err)
		}
		migUUID := fmt.Sprintf("MIG-%s", GenerateUuid())
		mappingInfo = append(mappingInfo, MigDeviceMappingInfo{
			Position:      migDevice.Position,
			DeviceUUID:    migUUID,
			GpuInstanceId: gpuInstanceId,
		})
		topologyInstances = append(topologyInstances, topology.MigInstance{
			Profile: migDevice.Name,
			Index:   migDevice.Position,
			UUID:    migUUID,
			Status: topology.GpuStatus{
				PodGpuUsageStatus: topology.PodGpuUsageStatusMap{},
			},
		})
	}

	return mappingInfo, topologyInstances, nil
}

func expandGPUIndices(devices []string, gpuCount int) ([]int, error) {
	gpuIndices := []int{}
	seen := map[int]bool{}

	for _, device := range devices {
		if device == "all" {
			for idx := 0; idx < gpuCount; idx++ {
				if !seen[idx] {
					gpuIndices = append(gpuIndices, idx)
					seen[idx] = true
				}
			}
			continue
		}

		gpuIdx, err := strconv.Atoi(device)
		if err != nil {
			return nil, fmt.Errorf("failed to parse gpu index %s: %w", device, err)
		}
		if gpuIdx < 0 || gpuIdx >= gpuCount {
			return nil, fmt.Errorf("gpu index %d out of range for %d GPUs", gpuIdx, gpuCount)
		}
		if !seen[gpuIdx] {
			gpuIndices = append(gpuIndices, gpuIdx)
			seen[gpuIdx] = true
		}
	}

	return gpuIndices, nil
}

func (faker *MigFaker) getNodeTopology() (*topology.NodeTopology, error) {
	if faker.clientset == nil {
		return nil, fmt.Errorf("kubernetes client is nil")
	}

	nodeName := viper.GetString(constants.EnvNodeName)
	nodeTopology, err := topology.GetNodeTopologyFromCM(faker.clientset, nodeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get node topology: %w", err)
	}

	return nodeTopology, nil
}

func (faker *MigFaker) updateNodeTopology(nodeTopology *topology.NodeTopology, migInstances map[int][]topology.MigInstance) error {
	for idx := range nodeTopology.Gpus {
		instances := migInstances[idx]
		nodeTopology.Gpus[idx].MigEnabled = len(instances) > 0
		nodeTopology.Gpus[idx].MigInstances = instances
	}

	nodeName := viper.GetString(constants.EnvNodeName)
	if err := topology.UpdateNodeTopologyCM(faker.clientset, nodeTopology, nodeName); err != nil {
		return fmt.Errorf("failed to update node topology: %w", err)
	}

	return nil
}

func (faker *MigFaker) restartDevicePluginPod() error {
	namespace := viper.GetString(constants.EnvTopologyCmNamespace)
	nodeName := viper.GetString(constants.EnvNodeName)
	selector := labels.Set{"app": "device-plugin", "component": "device-plugin"}.String()
	pods, err := faker.clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return fmt.Errorf("failed to list device-plugin pods: %w", err)
	}

	for _, pod := range pods.Items {
		if pod.Spec.NodeName != nodeName {
			continue
		}
		log.Printf("Deleting device-plugin pod %s/%s to reload MIG resources", namespace, pod.Name)
		if err := faker.clientset.CoreV1().Pods(namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete device-plugin pod %s/%s: %w", namespace, pod.Name, err)
		}
	}

	return nil
}

func (faker *MigFaker) getGpuProduct() (string, error) {
	nodeLabels, err := faker.kubeclient.GetNodeLabels()
	if err != nil {
		return "", fmt.Errorf("failed to get node labels: %w", err)
	}

	return nodeLabels[constants.LabelGpuProduct], nil
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
