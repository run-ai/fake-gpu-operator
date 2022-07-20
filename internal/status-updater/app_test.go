package status_updater_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	kfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	status_updater "github.com/run-ai/fake-gpu-operator/internal/status-updater"
)

const (
	topologyCmName              = "fake-cm-name"
	topologyCmNamespace         = "fake-cm-namespace"
	podNamespace                = "fake-pod-namespace"
	podName                     = "fake-pod-name"
	podUID                      = "fake-pod-uid"
	containerName               = "fake-container-name"
	reservationPodNs            = "runai-reservation"
	reservationPodName          = "gpu-reservation-pod"
	reservationPodContainerName = "reservation-pod-container"
	podGroupName                = "pg"
	node                        = "fake-node"
	nodeGpuCount                = 2
)

func TestStatusUpdater(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "StatusUpdater Suite")
}

var _ = Describe("StatusUpdater", func() {
	var (
		kubeclient    kubernetes.Interface
		dynamicClient dynamic.Interface
		app           *status_updater.App
	)

	BeforeEach(func() {
		clusterTopology := &topology.ClusterTopology{
			MigStrategy: "mixed",
			Nodes: map[string]topology.NodeTopology{
				node: {
					GpuMemory:  11441,
					GpuCount:   nodeGpuCount,
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

		kubeclient = kfake.NewSimpleClientset()
		scheme := runtime.NewScheme()
		scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "scheduling.run.ai", Version: "v1", Kind: "PodGroup"}, &unstructured.UnstructuredList{})
		dynamicClient = dfake.NewSimpleDynamicClient(scheme)

		_, err = kubeclient.CoreV1().ConfigMaps(topologyCmNamespace).Create(context.TODO(), topologyConfigMap, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())
		setupFakes(kubeclient, dynamicClient)
		setupConfig()

		app = status_updater.NewApp()
		go app.Run()
	})

	AfterEach(func() {
		app.Stop()
	})

	When("the status updater is started", func() {
		It("should reset the cluster topology", func() {
			Eventually(getTopologyFromKube(kubeclient)).Should(Equal(createTopology(nodeGpuCount)))
		})
	})

	When("informed of a dedicated GPU pod", func() {
		type testCase struct {
			podGpuCount  int64
			podPhase     v1.PodPhase
			workloadType string
		}

		cases := []testCase{}

		for i := int64(1); i <= nodeGpuCount; i++ {
			for _, phase := range []v1.PodPhase{v1.PodPending, v1.PodRunning, v1.PodSucceeded, v1.PodFailed, v1.PodUnknown} {
				for _, workloadType := range []string{"build", "train", "interactive-preemptible", "inference"} {
					cases = append(cases, testCase{
						podGpuCount:  i,
						podPhase:     phase,
						workloadType: workloadType,
					})
				}
			}
		}

		for _, caseDetails := range cases {
			caseBaseName := fmt.Sprintf("GPU count %d, pod phase %s, workloadType: %s", caseDetails.podGpuCount, caseDetails.podPhase, caseDetails.workloadType)
			caseDetails := caseDetails
			It(caseBaseName, func() {
				pod := createDedicatedGpuPod(caseDetails.podGpuCount, caseDetails.podPhase)
				_, err := kubeclient.CoreV1().Pods(podNamespace).Create(context.TODO(), pod, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
				podGroup := createPodGroup(caseDetails.workloadType)
				_, err = dynamicClient.Resource(schema.GroupVersionResource{Group: "scheduling.run.ai", Version: "v1", Resource: "podgroups"}).Namespace(podNamespace).Create(context.TODO(), podGroup, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				expectedTopology := createTopology(nodeGpuCount)
				if caseDetails.podPhase == v1.PodRunning {
					for i := 0; i < int(caseDetails.podGpuCount); i++ {
						expectedTopology.Nodes[node].Gpus[i].Status.PodGpuUsageStatus = topology.PodGpuUsageStatusMap{
							podUID: topology.GpuUsageStatus{
								Utilization:    getWorkloadTypeExpectedUtilization(caseDetails.workloadType),
								FbUsed:         expectedTopology.Nodes[node].GpuMemory,
								IsInferencePod: caseDetails.workloadType == "inference",
							},
						}
						expectedTopology.Nodes[node].Gpus[i].Status.AllocatedBy.Pod = podName
						expectedTopology.Nodes[node].Gpus[i].Status.AllocatedBy.Container = containerName
						expectedTopology.Nodes[node].Gpus[i].Status.AllocatedBy.Namespace = podNamespace
					}
				}

				Eventually(getTopologyFromKube(kubeclient)).Should(Equal(expectedTopology))

				err = kubeclient.CoreV1().Pods(podNamespace).Delete(context.TODO(), podName, metav1.DeleteOptions{})
				Expect(err).ToNot(HaveOccurred())
				Eventually(getTopologyFromKube(kubeclient)).Should(Equal(createTopology(nodeGpuCount)))
			})
		}
	})

	When("informed of a shared GPU pod", func() {
		It("should update the cluster topology at its reservation pod location", func() {
			// Test reservation pod handling
			reservationPod := createReservationPod(0)
			_, err := kubeclient.CoreV1().Pods(reservationPodNs).Create(context.TODO(), reservationPod, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			expectedTopology := createTopology(nodeGpuCount)
			expectedTopology.Nodes[node].Gpus[0].Status.AllocatedBy.Pod = reservationPodName
			expectedTopology.Nodes[node].Gpus[0].Status.AllocatedBy.Container = reservationPodContainerName
			expectedTopology.Nodes[node].Gpus[0].Status.AllocatedBy.Namespace = reservationPodNs
			Eventually(getTopologyFromKube(kubeclient)).Should(Equal(expectedTopology))

			// Test shared gpu pod handling
			pod := createSharedGpuPod(0, 0.5, v1.PodRunning)
			_, err = kubeclient.CoreV1().Pods(podNamespace).Create(context.TODO(), pod, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			podGroup := createPodGroup("train")
			_, err = dynamicClient.Resource(schema.GroupVersionResource{Group: "scheduling.run.ai", Version: "v1", Resource: "podgroups"}).Namespace(podNamespace).Create(context.TODO(), podGroup, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			expectedTopology.Nodes[node].Gpus[0].Status.PodGpuUsageStatus = topology.PodGpuUsageStatusMap{
				podUID: topology.GpuUsageStatus{
					Utilization: topology.Range{
						Min: 80,
						Max: 100,
					},
					FbUsed: int(float64(expectedTopology.Nodes[node].GpuMemory) * 0.5),
				},
			}

			Eventually(getTopologyFromKube(kubeclient)).Should(Equal(expectedTopology))
		})
	})
})

func getTopologyFromKube(kubeclient kubernetes.Interface) func() (*topology.ClusterTopology, error) {
	return func() (*topology.ClusterTopology, error) {
		ret, err := topology.GetFromKube(kubeclient)
		return ret, err
	}
}

func setupFakes(kubeclient kubernetes.Interface, dynamicClient dynamic.Interface) {
	status_updater.InClusterConfigFn = func() (*rest.Config, error) {
		return nil, nil
	}
	status_updater.KubeClientFn = func(c *rest.Config) kubernetes.Interface {
		return kubeclient
	}
	status_updater.DynamicClientFn = func(c *rest.Config) dynamic.Interface {
		return dynamicClient
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
			Status: topology.GpuStatus{
				PodGpuUsageStatus: topology.PodGpuUsageStatusMap{},
			},
		}
	}

	return &topology.ClusterTopology{
		MigStrategy: "mixed",
		Nodes: map[string]topology.NodeTopology{
			node: {
				GpuMemory:  11441,
				GpuCount:   int(gpuCount),
				GpuProduct: "Tesla-K80",
				Gpus:       gpus,
			},
		},
	}
}

func createDedicatedGpuPod(gpuCount int64, phase v1.PodPhase) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
			UID:       podUID,
			Annotations: map[string]string{
				"pod-group-name": podGroupName,
			},
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

func createSharedGpuPod(gpuIdx int, gpuFraction float64, phase v1.PodPhase) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
			UID:       podUID,
			Annotations: map[string]string{
				"runai-gpu":      strconv.Itoa(gpuIdx),
				"gpu-fraction":   fmt.Sprintf("%f", gpuFraction),
				"pod-group-name": podGroupName,
			},
		},
		Spec: v1.PodSpec{
			NodeName: node,
			Containers: []v1.Container{
				{
					Name: containerName,
				},
			},
		},
		Status: v1.PodStatus{
			Phase: phase,
		},
	}
}

func createReservationPod(gpuIdx int) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      reservationPodName,
			Namespace: reservationPodNs,
			Annotations: map[string]string{
				"run.ai/reserve_for_gpu_index": strconv.Itoa(gpuIdx),
			},
		},
		Spec: v1.PodSpec{
			NodeName: node,
			Containers: []v1.Container{
				{
					Name: reservationPodContainerName,
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							"nvidia.com/gpu": *resource.NewQuantity(1, resource.DecimalSI),
						},
					},
				},
			},
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}
}

func createPodGroup(workloadType string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "scheduling.run.ai/v1",
			"kind":       "PodGroup",
			"metadata": map[string]interface{}{
				"name":      podGroupName,
				"namespace": podNamespace,
			},
			"spec": map[string]interface{}{
				"priorityClassName": workloadType,
			},
		},
	}
}

func getWorkloadTypeExpectedUtilization(workloadType string) topology.Range {
	switch workloadType {
	case "train":
		return topology.Range{
			Min: 80,
			Max: 100,
		}
	case "build", "interactive-preemptible", "inference":
		return topology.Range{
			Min: 0,
			Max: 0,
		}
	default:
		return topology.Range{
			Min: 100,
			Max: 100,
		}
	}
}
