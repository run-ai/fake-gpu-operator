package deviceplugin

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/net/context"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var _ = Describe("FakeNodeDevicePlugin.Serve", func() {
	It("should update the node capacity and allocatable", func() {
		node := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node1",
			},
		}
		err := os.Setenv("NODE_NAME", "node1")
		Expect(err).ToNot(HaveOccurred())

		fakeClient := fake.NewSimpleClientset(node)

		fakeNodeDevicePlugin := &FakeNodeDevicePlugin{
			kubeClient:   fakeClient,
			gpuCount:     1,
			otherDevices: map[string]int{"device1": 2},
		}

		err = fakeNodeDevicePlugin.Serve()
		Expect(err).ToNot(HaveOccurred())

		updateNode, err := fakeClient.CoreV1().Nodes().Get(context.TODO(), "node1", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(testResourceListCondition(updateNode.Status.Capacity, v1.ResourceName(nvidiaGPUResourceName), 1)).To(BeTrue())
		Expect(testResourceListCondition(updateNode.Status.Allocatable, v1.ResourceName(nvidiaGPUResourceName), 1)).To(BeTrue())
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
