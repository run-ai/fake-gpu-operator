package node

import (
	"context"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/spf13/viper"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	testcore "k8s.io/client-go/testing"
)

var _ = Describe("NodeHandler", func() {
	const (
		nodeName = "test-node"
		fgoNS    = "gpu-operator"
	)
	var (
		nodeHandler *NodeHandler
		kubeClient  *fake.Clientset
		watcher     *watch.FakeWatcher

		clusterTopology *topology.ClusterTopology
		ctx             = context.Background()
	)

	BeforeEach(func() {
		// Set up environment variables
		os.Setenv(constants.EnvFakeGpuOperatorNs, fgoNS)
		os.Setenv(constants.EnvTopologyCmName, "topology")
		os.Setenv(constants.EnvTopologyCmNamespace, fgoNS)
		viper.AutomaticEnv()

		// Set up cluster topology
		clusterTopology = &topology.ClusterTopology{
			NodePoolLabelKey: "nodepool",
			NodePools: map[string]topology.NodePoolTopology{
				"default": {
					GpuMemory:  100,
					GpuProduct: "Tesla V100",
					GpuCount:   2,
				},
			},
		}

		// Set up kube client and node handler
		kubeClient = fake.NewSimpleClientset()
		watcher = watch.NewFakeWithChanSize(1, false)
		kubeClient.PrependWatchReactor("pods", testcore.DefaultWatchReactor(watcher, nil))
		nodeHandler = NewNodeHandler(kubeClient, clusterTopology)

		// Create DCGM Exporter Deployment Template
		_, err := kubeClient.AppsV1().Deployments(fgoNS).Create(ctx, &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "nvidia-dcgm-exporter",
				Labels: map[string]string{
					constants.LabelFakeNodeDeploymentTemplate: "true",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{},
				},
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							constants.LabelApp: constants.DCGMExporterApp,
						},
					},
					Spec: v1.PodSpec{
						Containers: []v1.Container{
							{
								Env: []v1.EnvVar{},
							},
						},
					},
				},
			},
		}, metav1.CreateOptions{})
		Expect(err).To(BeNil())
	})

	Context("HandleAdd", func() {
		It("should handle fake node addition", func() {
			node := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodeName,
					Annotations: map[string]string{
						constants.AnnotationKwokNode: "fake",
					},
					Labels: map[string]string{
						clusterTopology.NodePoolLabelKey: "default",
					},
				},
			}
			_, err := kubeClient.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
			Expect(err).To(BeNil())

			// Create DCGM Exporter Pod
			dcgmExporterPod := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nvidia-dcgm-exporter-123456",
					Namespace: fgoNS,
					Labels: map[string]string{
						constants.LabelApp: constants.DCGMExporterApp,
					},
				},
				Spec: v1.PodSpec{
					NodeName: nodeName,
				},
				Status: v1.PodStatus{
					PodIP: "10.0.0.1",
					Phase: v1.PodRunning,
				},
			}
			_, err = kubeClient.CoreV1().Pods(fgoNS).Create(ctx, dcgmExporterPod, metav1.CreateOptions{})
			Expect(err).To(BeNil())
			watcher.Add(dcgmExporterPod)

			// Watch for dcgmExporterPod creation
			// podWatcher, err := kubeClient.CoreV1().Pods(viper.GetString(constants.EnvFakeGpuOperatorNs)).Watch(ctx, metav1.ListOptions{
			// 	LabelSelector: "app=dcgm-exporter",
			// 	FieldSelector: "spec.nodeName=test-node",
			// })
			// Expect(err).To(BeNil())
			// defer podWatcher.Stop()
			// Eventually(podWatcher.ResultChan()).Should(Receive())

			err = nodeHandler.HandleAdd(node)
			Expect(err).To(BeNil())

			By("creating node topology ConfigMap")
			_, err = kubeClient.CoreV1().ConfigMaps(fgoNS).Get(ctx, topology.GetNodeTopologyCMName(node.Name), metav1.GetOptions{})
			Expect(err).To(BeNil())

			By("labeling node")
			node, err = kubeClient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
			Expect(err).To(BeNil())
			Expect(node.Labels).To(HaveKeyWithValue(dcgmExporterLabelKey, "true"))
			Expect(node.Labels).ToNot(HaveKeyWithValue(devicePluginLabelKey, "true"))

			By("applying fake node deployments")
			deployments, err := kubeClient.AppsV1().Deployments(fgoNS).List(ctx, metav1.ListOptions{
				LabelSelector: fmt.Sprintf("%s=true", constants.LabelFakeNodeDeployment),
			})
			Expect(err).To(BeNil())
			Expect(deployments.Items).To(HaveLen(1))
		})
	})
})
