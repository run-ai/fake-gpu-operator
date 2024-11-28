package configmap

import (
	"testing"

	"gopkg.in/yaml.v3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"golang.org/x/net/context"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestKWOKGPUDevicePlugin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "KWOK GPU DevicePlugin Suite")
}

var _ = Describe("HandleAdd", func() {
	It("should update the node capacity and allocatable", func() {
		nodeName := "node1"
		nodeTopology := &topology.NodeTopology{
			Gpus: []topology.GpuDetails{
				{ID: "0"},
			},
			OtherDevices: []topology.GenericDevice{
				{Name: "device1", Count: 2},
			},
		}

		topologyData, err := yaml.Marshal(nodeTopology)
		if err != nil {
			Fail("Failed to marshal topology data")
		}

		configMap := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
				Labels: map[string]string{
					constants.LabelTopologyCMNodeName: nodeName,
				},
			},
			Data: map[string]string{
				topology.CmTopologyKey: string(topologyData),
			},
		}

		node := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: nodeName,
			},
		}

		fakeClient := fake.NewSimpleClientset(node, configMap)

		fakeNodeCMHandler := NewConfigMapHandler(fakeClient, nil)
		err = fakeNodeCMHandler.HandleAdd(configMap)
		Expect(err).ToNot(HaveOccurred())

		updateNode, err := fakeClient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(testResourceListCondition(updateNode.Status.Capacity, v1.ResourceName(constants.GpuResourceName), 1)).To(BeTrue())
		Expect(testResourceListCondition(updateNode.Status.Allocatable, v1.ResourceName(constants.GpuResourceName), 1)).To(BeTrue())
		Expect(testResourceListCondition(updateNode.Status.Capacity, v1.ResourceName("device1"), 2)).To(BeTrue())
		Expect(testResourceListCondition(updateNode.Status.Allocatable, v1.ResourceName("device1"), 2)).To(BeTrue())
	})
})

func testResourceListCondition(resourceList v1.ResourceList, resourceName v1.ResourceName, value int64) bool {
	quantity, found := resourceList[resourceName]
	if !found {
		return false
	}
	return quantity.Value() == value
}
