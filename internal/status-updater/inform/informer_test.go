package inform_test

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/inform"
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
		informer   *inform.Informer
		pod        *v1.Pod
		ch         chan *inform.PodEvent
		stopCh     chan struct{}
	)

	BeforeEach(func() {
		kubeclient = fake.NewSimpleClientset()
		wg := &sync.WaitGroup{}
		wg.Add(1)
		informer = inform.NewInformer(kubeclient, wg)
		ch = make(chan *inform.PodEvent)
		informer.Subscribe(ch)
		stopCh = make(chan struct{})
		go func() {
			// Somehow the informer may not catch events if it is starting in the same
			// time as the events submission, so we wait.
			time.Sleep(50 * time.Millisecond)
			informer.Run(stopCh)
		}()
	})

	AfterEach(func() {
		close(stopCh)
		<-ch
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
				_, err := kubeclient.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
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
						_, err := kubeclient.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
						Expect(err).ToNot(HaveOccurred())
						Eventually(ch).Should(BeEmpty())
					})
				})

				When("the pod is running", func() {
					BeforeEach(func() {
						pod.Status.Phase = v1.PodRunning
					})

					It("should publish event", func() {
						_, err := kubeclient.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
						Expect(err).NotTo(HaveOccurred())
						Eventually(ch).Should(Receive(Equal(&inform.PodEvent{pod, inform.ADD})))
					})
				})
			})

			Context("pod is updated", func() {
				When("Running status is not changed", func() {
					It("should not publish event", func() {
						pod.Status.Phase = v1.PodRunning
						_, err := kubeclient.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
						Expect(err).ToNot(HaveOccurred())
						Eventually(ch).Should(Receive(Equal(&inform.PodEvent{pod, inform.ADD})))
						_, err = kubeclient.CoreV1().Pods("default").Update(context.TODO(), pod, metav1.UpdateOptions{})
						Expect(err).ToNot(HaveOccurred())
						Eventually(ch).Should(BeEmpty())
					})
				})

				When("Running status is changed", func() {
					When("the old pod isn't running and the new pod is running", func() {
						It("should publish ADD event", func() {
							pod.Status.Phase = v1.PodPending
							_, err := kubeclient.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
							Expect(err).To(Not(HaveOccurred()))
							pod.Status.Phase = v1.PodRunning
							_, err = kubeclient.CoreV1().Pods("default").Update(context.TODO(), pod, metav1.UpdateOptions{})
							Expect(err).To(Not(HaveOccurred()))
							Eventually(ch).Should(Receive(Equal(&inform.PodEvent{pod, inform.ADD})))
						})
					})
					When("the old pod is running and the new pod isn't running", func() {
						It("should publish DELETE event", func() {
							pod.Status.Phase = v1.PodRunning
							_, err := kubeclient.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
							Expect(err).To(Not(HaveOccurred()))
							Eventually(ch).Should(Receive(Equal(&inform.PodEvent{pod, inform.ADD})))
							pod.Status.Phase = v1.PodPending
							_, err = kubeclient.CoreV1().Pods("default").Update(context.TODO(), pod, metav1.UpdateOptions{})
							Expect(err).To(Not(HaveOccurred()))
							Eventually(ch).Should(Receive(Equal(&inform.PodEvent{pod, inform.DELETE})))
						})
					})
				})

			})

			Context("pod is deleted", func() {
				When("the pod was running", func() {
					It("should publish DELETE event", func() {
						pod.Status.Phase = v1.PodRunning
						_, err := kubeclient.CoreV1().Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
						Expect(err).ToNot(HaveOccurred())
						Eventually(ch).Should(Receive(Equal(&inform.PodEvent{pod, inform.ADD})))
						err = kubeclient.CoreV1().Pods("default").Delete(context.TODO(), pod.Name, metav1.DeleteOptions{})
						Expect(err).ToNot(HaveOccurred())
						Eventually(ch).Should(Receive(Equal(&inform.PodEvent{pod, inform.DELETE})))
					})
				})
			})
		})
	})
})
