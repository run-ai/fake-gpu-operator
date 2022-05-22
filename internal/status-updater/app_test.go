package status_updater_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/run-ai/gpu-mock-stack/internal/common/topology"
	status_updater "github.com/run-ai/gpu-mock-stack/internal/status-updater"
)

const (
	topologyCmName      = "fake-cm-name"
	topologyCmNamespace = "fake-cm-namespace"
	podNamespace        = "fake-pod-namespace"
	podName             = "fake-pod-name"
	containerName       = "fake-container-name"
	node                = "fake-node"
	gpuCount            = 2
)

func TestStatusUpdater(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "StatusUpdater Suite")
}

var _ = Describe("StatusUpdater", func() {
	var (
		kubeclient kubernetes.Interface
	)

	BeforeEach(func() {
		clusterTopology := &topology.ClusterTopology{
			MigStrategy: "mixed",
			Nodes: map[string]topology.NodeTopology{
				node: {
					GpuMemory:  11441,
					GpuCount:   gpuCount,
					GpuProduct: "Tesla-K80",
				},
			},
		}

		topologyStr, err := yaml.Marshal(clusterTopology)
		Expect(err).ToNot(HaveOccurred())
		topologyConfigMap := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      topologyCmName,
				Namespace: topologyCmNamespace,
			},
			Data: map[string]string{
				"topology.yml": string(topologyStr),
			},
		}

		kubeclient = fake.NewSimpleClientset()

		kubeclient.CoreV1().ConfigMaps(topologyCmNamespace).Create(context.TODO(), topologyConfigMap, metav1.CreateOptions{})
		setupFakes(kubeclient)
		setupConfig()
	})

	When("the status updater is started", func() {
		It("should reset the cluster topology", func() {
			go status_updater.Run()
			Eventually(getTopologyFromKube(kubeclient)).Should(Equal(createTopology(gpuCount)))
		})
	})

	When("GPU pod is added", func() {
		type testCase struct {
			gpuCount int64
			podPhase v1.PodPhase
		}
		cases := []testCase{}

		for i := int64(0); i < gpuCount; i++ {
			for _, phase := range []v1.PodPhase{v1.PodPending, v1.PodRunning, v1.PodSucceeded, v1.PodFailed, v1.PodUnknown} {
				cases = append(cases, testCase{
					gpuCount: i,
					podPhase: phase,
				})
			}
		}

		for _, caseDetails := range cases {
			caseName := fmt.Sprintf("GPU count %d, pod phase %s", caseDetails.gpuCount, caseDetails.podPhase)
			It(caseName, func() {
				pod := createPod(caseDetails.gpuCount, caseDetails.podPhase)
				kubeclient.CoreV1().Pods(podNamespace).Create(context.TODO(), pod, metav1.CreateOptions{})

				expectedTopology := createTopology(gpuCount)
				if caseDetails.podPhase == v1.PodRunning {
					for i := 0; i < int(caseDetails.gpuCount); i++ {
						expectedTopology.Nodes[node].Gpus[i].Metrics.Status.Utilization = 100
						expectedTopology.Nodes[node].Gpus[i].Metrics.Status.FbUsed = expectedTopology.Nodes[node].GpuMemory
						expectedTopology.Nodes[node].Gpus[i].Metrics.Metadata.Pod = podName
						expectedTopology.Nodes[node].Gpus[i].Metrics.Metadata.Container = containerName
						expectedTopology.Nodes[node].Gpus[i].Metrics.Metadata.Namespace = podNamespace
					}
				}

				go status_updater.Run()
				Eventually(getTopologyFromKube(kubeclient)).Should(Equal(expectedTopology))
			})
		}
	})
})

func getTopologyFromKube(kubeclient kubernetes.Interface) func() (*topology.ClusterTopology, error) {
	return func() (*topology.ClusterTopology, error) {
		return topology.GetFromKube(kubeclient)
	}
}

func setupFakes(kubeclient kubernetes.Interface) {
	status_updater.InClusterConfigFn = func() (*rest.Config, error) {
		return nil, nil
	}
	status_updater.KubeClientFn = func(c *rest.Config) kubernetes.Interface {
		return kubeclient
	}
}

func setupConfig() {
	setupEnvs()
}

func setupEnvs() {
	os.Setenv("TOPOLOGY_CM_NAME", "fake-cm-name")
	os.Setenv("TOPOLOGY_CM_NAMESPACE", "fake-cm-namespace")
}

func createTopology(gpuCount int64) *topology.ClusterTopology {
	gpus := make([]topology.GpuDetails, gpuCount)
	for i := int64(0); i < gpuCount; i++ {
		gpus[i] = topology.GpuDetails{
			ID: fmt.Sprintf("gpu-%d", i),
		}
	}

	return &topology.ClusterTopology{
		MigStrategy: "mixed",
		Nodes: map[string]topology.NodeTopology{
			node: {
				GpuMemory:  11441,
				GpuCount:   2,
				GpuProduct: "Tesla-K80",
				Gpus:       gpus,
			},
		},
	}
}

func createPod(gpuCount int64, phase v1.PodPhase) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		Spec: v1.PodSpec{
			NodeName: node,
			Containers: []v1.Container{
				{
					Name: containerName,
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							"nvidia.com/gpu": *resource.NewQuantity(gpuCount, resource.DecimalSI),
						},
					},
				},
			},
		},
		Status: v1.PodStatus{
			Phase: phase,
		},
	}
}
