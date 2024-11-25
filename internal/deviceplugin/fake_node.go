package deviceplugin

import (
	"fmt"
	"os"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type FakeNodeDevicePlugin struct {
	kubeClient kubernetes.Interface
	gpuCount   int
}

func (f *FakeNodeDevicePlugin) Serve() error {
	patch := fmt.Sprintf(`{"status": {"capacity": {"%s": "%d"}, "allocatable": {"%s": "%d"}}}`, nvidiaGPUResourceName, f.gpuCount, nvidiaGPUResourceName, f.gpuCount)
	_, err := f.kubeClient.CoreV1().Nodes().Patch(context.TODO(), os.Getenv(constants.EnvNodeName), types.MergePatchType, []byte(patch), metav1.PatchOptions{}, "status")
	if err != nil {
		return fmt.Errorf("failed to update node capacity and allocatable: %v", err)
	}

	return nil
}

func (f *FakeNodeDevicePlugin) Name() string {
	return "FakeNodeDevicePlugin"
}
