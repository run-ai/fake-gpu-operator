package kwokgdp

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	cmcontroller "github.com/run-ai/fake-gpu-operator/internal/kwok-gpu-device-plugin/controllers/configmap"
	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers"
)

const (
	gpuOperatorNamespace = "gpu-operator"
	nodePoolLabelKey     = "run.ai/node-pool"
	defaultNodePoolName  = "default"
)

func TestKwokGpuDevicePlugin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "KwokGpuDevicePlugin Suite")
}

var _ = Describe("KwokGpuDevicePlugin", func() {
	var (
		app        *KWOKDevicePluginApp
		kubeClient kubernetes.Interface
		stopChan   chan struct{}
		wg         *sync.WaitGroup
	)

	BeforeEach(func() {
		clusterTopology := topology.ClusterTopology{
			NodePoolLabelKey: nodePoolLabelKey,
			NodePools: map[string]topology.NodePoolTopology{
				defaultNodePoolName: {
					GpuCount:   4,
					GpuMemory:  1000,
					GpuProduct: "nvidia-tesla-t4",
				},
			},
			MigStrategy: "none",
		}
		clusterTopologyCM, err := topology.ToClusterTopologyCM(&clusterTopology)
		Expect(err).ToNot(HaveOccurred())
		clusterTopologyCM.Name = "cluster-topology"
		clusterTopologyCM.Namespace = gpuOperatorNamespace

		kubeClient = fake.NewSimpleClientset(clusterTopologyCM)
		stopChan = make(chan struct{})

		viper.SetDefault(constants.EnvTopologyCmName, clusterTopologyCM.Name)
		viper.SetDefault(constants.EnvTopologyCmNamespace, gpuOperatorNamespace)

		app = &KWOKDevicePluginApp{
			Controllers: []controllers.Interface{
				cmcontroller.NewConfigMapController(
					kubeClient, gpuOperatorNamespace,
				),
			},
			kubeClient: kubeClient,
			stopCh:     stopChan,
		}
		wg = &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			app.Run()
			wg.Done()
		}()
	})

	AfterEach(func() {
		close(stopChan)
		wg.Wait()
	})

	Context("app", func() {
		It("should run until channel is closed", func() {})

		Context("ConfigMap", func() {
			It("should handle new Config Map without node labels", func() {
				_, err := kubeClient.CoreV1().ConfigMaps(gpuOperatorNamespace).Create(context.TODO(), &v1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "configmap1",
						Namespace: gpuOperatorNamespace,
					},
				}, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should add gpu devices to kwok nodes by configmap data", func() {
				node1 := &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							nodePoolLabelKey: defaultNodePoolName,
						},
						Annotations: map[string]string{
							constants.AnnotationKwokNode: "fake",
						},
					},
				}
				_, err := kubeClient.CoreV1().Nodes().Create(context.TODO(), node1, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				nodeTopology := topology.NodeTopology{
					GpuMemory:  1000,
					GpuProduct: "nvidia-tesla-t4",
					Gpus: []topology.GpuDetails{
						{ID: "fake-gpu-id-1", Status: topology.GpuStatus{}},
						{ID: "fake-gpu-id-2", Status: topology.GpuStatus{}},
						{ID: "fake-gpu-id-3", Status: topology.GpuStatus{}},
						{ID: "fake-gpu-id-4", Status: topology.GpuStatus{}},
					},
				}
				cm, err := topology.ToNodeTopologyCM(&nodeTopology, node1.Name)
				Expect(err).ToNot(HaveOccurred())
				cm.Namespace = gpuOperatorNamespace

				_, err = kubeClient.CoreV1().ConfigMaps(gpuOperatorNamespace).Create(context.TODO(), cm, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() bool {
					node, err := kubeClient.CoreV1().Nodes().Get(context.TODO(), node1.Name, metav1.GetOptions{})
					if err != nil {
						return false
					}

					gpuQuantity := node.Status.Capacity[constants.GpuResourceName]
					return gpuQuantity.Value() == int64(4)
				}, 2*time.Second, 100*time.Millisecond).Should(BeTrue())
			})
		})
	})
})
