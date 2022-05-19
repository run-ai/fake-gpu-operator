package inform_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/run-ai/gpu-mock-stack/internal/status-updater/inform"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestInformer(t *testing.T) {
	format.MaxLength = 10000
	RegisterFailHandler(Fail)
	RunSpecs(t, "Informer Suite")
}

var _ = Describe("Informer", func() {
	const (
		nvidiaGpuResourceName = "nvidia.com/gpu"
	)

	var (
		kubeclient *fake.Clientset
		inf        *inform.Informer
		pod        *v1.Pod
		ch         chan *inform.PodEvent
		stopCh     chan struct{}
	)

	BeforeEach(func() {
		kubeclient = fake.NewSimpleClientset()
		inf = inform.NewInformer(kubeclient)
		ch = make(chan *inform.PodEvent)
		inf.Subscribe(ch)
		stopCh = make(chan struct{})
		go inf.Run(stopCh)
	})

	AfterEach(func() {
		close(stopCh)
	})

	Context("pod event is received", func() {
		BeforeEach(func() {
			pod = &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "default",
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Resources: v1.ResourceRequirements{
								Limits: v1.ResourceList{},
							},
						},
					},
				},
			}
		})

		When("pod is not requesting GPUs", func() {
			It("should not publish event", func() {
				kubeclient.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
				Eventually(ch).Should(BeEmpty())
			})
		})

		When("pod is requesting GPUs", func() {
			BeforeEach(func() {
				pod.Spec.Containers[0].Resources.Limits[nvidiaGpuResourceName] = resource.MustParse("1")
			})

			Context("pod is added", func() {
				When("the pod is not running", func() {
					BeforeEach(func() {
						pod.Status.Phase = v1.PodPending
					})

					It("should not publish event", func() {
						kubeclient.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
						Eventually(ch).Should(BeEmpty())
					})
				})

				When("the pod is running", func() {
					BeforeEach(func() {
						pod.Status.Phase = v1.PodRunning
					})

					It("should publish event", func() {
						kubeclient.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
						Eventually(ch).Should(Receive(Equal(&inform.PodEvent{pod, inform.ADD})))
					})
				})
			})

			Context("pod is updated", func() {
				When("Running status is not changed", func() {
					It("should not publish event", func() {
						pod.Status.Phase = v1.PodRunning
						kubeclient.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
						Eventually(ch).Should(Receive(Equal(&inform.PodEvent{pod, inform.ADD})))
						kubeclient.CoreV1().Pods("default").Update(context.TODO(), pod, metav1.UpdateOptions{})
						Eventually(ch).Should(BeEmpty())
					})
				})

				When("Running status is changed", func() {
					When("the old pod isn't running and the new pod is running", func() {
						It("should publish ADD event", func() {
							pod.Status.Phase = v1.PodPending
							kubeclient.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
							pod.Status.Phase = v1.PodRunning
							kubeclient.CoreV1().Pods("default").Update(context.TODO(), pod, metav1.UpdateOptions{})
							Eventually(ch).Should(Receive(Equal(&inform.PodEvent{pod, inform.ADD})))
						})
					})
					When("the old pod is running and the new pod isn't running", func() {
						It("should publish DELETE event", func() {
							pod.Status.Phase = v1.PodRunning
							kubeclient.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
							Eventually(ch).Should(Receive(Equal(&inform.PodEvent{pod, inform.ADD})))
							pod.Status.Phase = v1.PodPending
							kubeclient.CoreV1().Pods("default").Update(context.TODO(), pod, metav1.UpdateOptions{})
							Eventually(ch).Should(Receive(Equal(&inform.PodEvent{pod, inform.DELETE})))
						})
					})
				})

			})

			Context("pod is deleted", func() {
				When("the pod was running", func() {
					It("should publish DELETE event", func() {
						pod.Status.Phase = v1.PodRunning
						kubeclient.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
						Eventually(ch).Should(Receive(Equal(&inform.PodEvent{pod, inform.ADD})))
						kubeclient.CoreV1().Pods("default").Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
						Eventually(ch).Should(Receive(Equal(&inform.PodEvent{pod, inform.DELETE})))
					})
				})
			})
		})
	})
})
