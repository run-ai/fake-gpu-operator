package deviceplugin

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"golang.org/x/net/context"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FakeNodeDevicePlugin struct {
	kubeClient   kubernetes.Interface
	gpuCount     int
	otherDevices map[string]int
}

func (f *FakeNodeDevicePlugin) Serve() error {
	nodeStatus := v1.NodeStatus{
		Capacity: v1.ResourceList{
			v1.ResourceName(nvidiaGPUResourceName): *resource.NewQuantity(int64(f.gpuCount), resource.DecimalSI),
		},
		Allocatable: v1.ResourceList{
			v1.ResourceName(nvidiaGPUResourceName): *resource.NewQuantity(int64(f.gpuCount), resource.DecimalSI),
		},
	}

	for deviceName, count := range f.otherDevices {
		nodeStatus.Capacity[v1.ResourceName(deviceName)] = *resource.NewQuantity(int64(count), resource.DecimalSI)
		nodeStatus.Allocatable[v1.ResourceName(deviceName)] = *resource.NewQuantity(int64(count), resource.DecimalSI)
	}

	// Convert the patch struct to JSON
	patchBytes, err := json.Marshal(v1.Node{Status: nodeStatus})
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %v", err)
	}

	// Apply the patch
	_, err = f.kubeClient.CoreV1().Nodes().Patch(context.TODO(), os.Getenv(constants.EnvNodeName), types.MergePatchType, patchBytes, metav1.PatchOptions{}, "status")
	if err != nil {
		return fmt.Errorf("failed to update node capacity and allocatable: %v", err)
	}

	return nil
}

func (f *FakeNodeDevicePlugin) Name() string {
	return "FakeNodeDevicePlugin"
}
