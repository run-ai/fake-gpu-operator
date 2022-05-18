package inform_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/run-ai/gpu-mock-stack/internal/status-updater/inform"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestInformer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Informer Suite")
}

var _ = Describe("Informer", func() {
	var (
		kubeclient *fake.Clientset
		inf        *inform.Informer
	)

	BeforeEach(func() {
		kubeclient = fake.NewSimpleClientset()
		inf = inform.NewInformer(kubeclient)
	})

	Context("pod event is received", func() {
		When("pod is not requesting GPUs", func() {
			It("should not publish event", func() {
				pod := &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pod-1",
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Resources: v1.ResourceRequirements{
									Limits: v1.ResourceList{
										v1.ResourceCPU:    resource.MustParse("1"),
										v1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
							},
						},
					},
				}

				ch := make(chan *inform.PodEvent)
				inf.Subscribe(ch)
				inf.Run(nil)

				kubeclient.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
				Eventually(ch).Should(BeEmpty())
			})
		})

	})
})
