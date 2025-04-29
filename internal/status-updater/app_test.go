package status_updater_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	"k8s.io/utils/ptr"

	"github.com/run-ai/fake-gpu-operator/internal/common/app"
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
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
	reservationPodName          = "gpu-reservation-pod"
	reservationPodContainerName = "reservation-pod-container"
	podGroupName                = "pg"
	node                        = "fake-node"
	nodeGpuCount                = 2
	resourceReservationNs       = "runai-reservation"
)

func TestStatusUpdater(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "StatusUpdater Suite")
}

var _ = Describe("StatusUpdater", func() {
	var (
		kubeclient    kubernetes.Interface
		dynamicClient dynamic.Interface
		appRunner     *app.AppRunner
		wg            *sync.WaitGroup
	)

	BeforeEach(func() {
		clusterTopology := &topology.ClusterTopology{
			NodePools: map[string]topology.NodePoolTopology{
				"default": {
					GpuMemory:  11441,
					GpuProduct: "Tesla-K80",
					GpuCount:   nodeGpuCount,
				},
			},
			NodePoolLabelKey: "run.ai/simulated-gpu-node-pool",
			MigStrategy:      "mixed",
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

		// Create a gpuNode
		gpuNode := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: node,
				Labels: map[string]string{
					"run.ai/simulated-gpu-node-pool": "default",
				},
			},
		}

		_, err = kubeclient.CoreV1().Nodes().Create(context.TODO(), gpuNode, metav1.CreateOptions{})
		Expect(err).ToNot(HaveOccurred())

		setupFakes(kubeclient, dynamicClient)
		setupConfig()

		appRunner = app.NewAppRunner(&status_updater.StatusUpdaterApp{})
		wg = &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			appRunner.Run()
		}()
		time.Sleep(100 * time.Millisecond)
	})

	AfterEach(func() {
		if appRunner != nil {
			appRunner.Stop()
			wg.Wait()
		}
	})

	When("the status updater is started", func() {
		It("should initialize the topology nodes", func() {
			Eventually(getTopologyNodeFromKube(kubeclient, node)).Should(Equal(createTopology(nodeGpuCount, node)))
		})
	})

	When("informed of a dedicated GPU pod", func() {
		Context("non-reservation pod", func() {
			Context("creation and deletion", func() {
				type testCase struct {
					podGpuCount   int64
					podPhase      v1.PodPhase
					podConditions []v1.PodCondition
					workloadType  string
				}

				cases := []testCase{}

				for i := int64(1); i <= nodeGpuCount; i++ {
					for _, phase := range []v1.PodPhase{v1.PodPending, v1.PodRunning, v1.PodSucceeded, v1.PodFailed, v1.PodUnknown} {
						for _, workloadType := range []string{"build", "train", "interactive-preemptible", "inference"} {
							tCase := testCase{
								podGpuCount:  i,
								podPhase:     phase,
								workloadType: workloadType,
							}

							if phase == v1.PodPending { // Pending pods can be unscheduled or scheduled (e.g. when scheduled but the containers are not started yet)
								tCase.podConditions = []v1.PodCondition{{Type: v1.PodScheduled, Status: v1.ConditionFalse}}
								cases = append(cases, tCase)

								tCase.podConditions = []v1.PodCondition{{Type: v1.PodScheduled, Status: v1.ConditionTrue}}
								cases = append(cases, tCase)
							} else { // Non-pending pods are always scheduled
								tCase.podConditions = []v1.PodCondition{{Type: v1.PodScheduled, Status: v1.ConditionTrue}}
								cases = append(cases, tCase)
							}
						}
					}
				}

				for _, caseDetails := range cases {
					caseBaseName := fmt.Sprintf("GPU count %d, pod phase %s, workloadType: %s", caseDetails.podGpuCount, caseDetails.podPhase, caseDetails.workloadType)
					caseDetails := caseDetails
					It(caseBaseName, func() {
						By("creating the pod")
						pod := createDedicatedGpuPod(caseDetails.podGpuCount, caseDetails.podPhase, caseDetails.podConditions)
						_, err := kubeclient.CoreV1().Pods(podNamespace).Create(context.TODO(), pod, metav1.CreateOptions{})
						Expect(err).ToNot(HaveOccurred())
						podGroup := createPodGroup(caseDetails.workloadType)
						_, err = dynamicClient.Resource(schema.GroupVersionResource{Group: "scheduling.run.ai", Version: "v1", Resource: "podgroups"}).Namespace(podNamespace).Create(context.TODO(), podGroup, metav1.CreateOptions{})
						Expect(err).ToNot(HaveOccurred())

						expectedTopology := createTopology(nodeGpuCount, node)
						isPodScheduledConditionTrue := isConditionTrue(caseDetails.podConditions, v1.PodScheduled)

						if caseDetails.podPhase == v1.PodRunning ||
							((caseDetails.podPhase == v1.PodPending || caseDetails.podPhase == v1.PodUnknown) && isPodScheduledConditionTrue) {
							for i := 0; i < int(caseDetails.podGpuCount); i++ {
								expectedTopology.Gpus[i].Status.PodGpuUsageStatus = topology.PodGpuUsageStatusMap{
									podUID: topology.GpuUsageStatus{
										Utilization:           getExpectedUtilization(caseDetails.workloadType, caseDetails.podPhase),
										FbUsed:                expectedTopology.GpuMemory,
										UseKnativeUtilization: caseDetails.workloadType == "inference" && caseDetails.podPhase == v1.PodRunning,
									},
								}
								expectedTopology.Gpus[i].Status.AllocatedBy.Pod = podName
								expectedTopology.Gpus[i].Status.AllocatedBy.Container = containerName
								expectedTopology.Gpus[i].Status.AllocatedBy.Namespace = podNamespace
							}
						}

						Eventually(getTopologyNodeFromKube(kubeclient, node)).Should(Equal(expectedTopology))

						By("deleting the pod")
						err = kubeclient.CoreV1().Pods(podNamespace).Delete(context.TODO(), podName, metav1.DeleteOptions{})
						Expect(err).ToNot(HaveOccurred())
						Eventually(getTopologyNodeFromKube(kubeclient, node)).Should(Equal(createTopology(nodeGpuCount, node)))
					})
				}
			})

			Context("update", func() {
				When("pod phase changes", func() {
					BeforeEach(func() {
						// Create a pod in the pending phase with scheduled condition true
						pod := createDedicatedGpuPod(1, v1.PodPending, []v1.PodCondition{{Type: v1.PodScheduled, Status: v1.ConditionTrue}})
						_, err := kubeclient.CoreV1().Pods(podNamespace).Create(context.TODO(), pod, metav1.CreateOptions{})
						Expect(err).ToNot(HaveOccurred())
						workloadType := "train"
						podGroup := createPodGroup(workloadType)
						_, err = dynamicClient.Resource(schema.GroupVersionResource{Group: "scheduling.run.ai", Version: "v1", Resource: "podgroups"}).Namespace(podNamespace).Create(context.TODO(), podGroup, metav1.CreateOptions{})
						Expect(err).ToNot(HaveOccurred())

						expectedTopology := createTopology(nodeGpuCount, node)
						expectedTopology.Gpus[0].Status.PodGpuUsageStatus = topology.PodGpuUsageStatusMap{
							podUID: topology.GpuUsageStatus{
								Utilization:           getExpectedUtilization(workloadType, v1.PodPending),
								FbUsed:                expectedTopology.GpuMemory,
								UseKnativeUtilization: false,
							},
						}

						By("creating the pod with pending phase")
						Eventually(getTopologyNodeFromKube(kubeclient, node)).Should(Equal(expectedTopology))

						By("updating the pod phase to running")
						pod, err = kubeclient.CoreV1().Pods(podNamespace).Get(context.TODO(), podName, metav1.GetOptions{})
						Expect(err).ToNot(HaveOccurred())
						pod.Status.Phase = v1.PodRunning
						_, err = kubeclient.CoreV1().Pods(podNamespace).UpdateStatus(context.TODO(), pod, metav1.UpdateOptions{})
						Expect(err).ToNot(HaveOccurred())

						expectedTopology.Gpus[0].Status.PodGpuUsageStatus[podUID] = topology.GpuUsageStatus{
							Utilization:           getExpectedUtilization(workloadType, v1.PodRunning),
							FbUsed:                expectedTopology.GpuMemory,
							UseKnativeUtilization: false,
						}

						Eventually(getTopologyNodeFromKube(kubeclient, node)).Should(Equal(expectedTopology))
					})
				})
			})
		})

		Context("reservation pod", func() {
			When("GPU Index annotation is set (pre 2.17)", func() {
				It("should not reset the GPU Index annotation", func() {
					gpuIdx := 0

					reservationPod := createGpuIdxReservationPod(ptr.To(gpuIdx))
					_, err := kubeclient.CoreV1().Pods(resourceReservationNs).Create(context.TODO(), reservationPod, metav1.CreateOptions{})
					Expect(err).ToNot(HaveOccurred())

					// Expect the pod to consistently contain the GPU Index annotation
					Consistently(func() (string, error) {
						pod, err := kubeclient.CoreV1().Pods(resourceReservationNs).Get(context.TODO(), reservationPodName, metav1.GetOptions{})
						if err != nil {
							return "", err
						}
						return pod.Annotations[constants.AnnotationReservationPodGpuIdx], nil
					}).Should(Equal(strconv.Itoa(gpuIdx)))
				})
			})

			When("Gpu Index annotation is not set (post 2.17)", func() {
				It("should set the GPU Index annotation with the GPU UUID", func() {
					reservationPod := createGpuIdxReservationPod(nil)
					_, err := kubeclient.CoreV1().Pods(resourceReservationNs).Create(context.TODO(), reservationPod, metav1.CreateOptions{})
					Expect(err).ToNot(HaveOccurred())

					Eventually(func() (bool, error) {
						pod, err := kubeclient.CoreV1().Pods(resourceReservationNs).Get(context.TODO(), reservationPodName, metav1.GetOptions{})
						if err != nil {
							return false, err
						}

						nodeTopology, err := getTopologyNodeFromKube(kubeclient, node)()
						if err != nil || nodeTopology == nil {
							return false, err
						}

						for _, gpuDetails := range nodeTopology.Gpus {
							if gpuDetails.ID == pod.Annotations[constants.AnnotationReservationPodGpuIdx] {
								return true, nil
							}
						}

						return false, nil
					}).Should(BeTrue())
				})
			})
		})
	})

	When("informed of a shared GPU pod", func() {
		var (
			expectedTopology *topology.NodeTopology
		)

		var (
			expectTopologyToBeUpdatedWithReservationPod = func() {
				expectedTopology = createTopology(nodeGpuCount, node)
				expectedTopology.Gpus[0].Status.AllocatedBy.Pod = reservationPodName
				expectedTopology.Gpus[0].Status.AllocatedBy.Container = reservationPodContainerName
				expectedTopology.Gpus[0].Status.AllocatedBy.Namespace = resourceReservationNs
				Eventually(getTopologyNodeFromKube(kubeclient, node)).Should(Equal(expectedTopology))
			}
			expectTopologyToBeUpdatedWithSharedGpuPod = func() {
				expectedTopology.Gpus[0].Status.PodGpuUsageStatus = topology.PodGpuUsageStatusMap{
					podUID: topology.GpuUsageStatus{
						Utilization: topology.Range{
							Min: 80,
							Max: 100,
						},
						FbUsed: int(float64(expectedTopology.GpuMemory) * 0.5),
					},
				}

				Eventually(getTopologyNodeFromKube(kubeclient, node)).Should(Equal(expectedTopology))
			}
		)

		Context("with a runai-gpu annotation", func() {
			It("should update the cluster topology at its reservation pod location", func() {
				gpuIdx := 0

				// Test reservation pod handling
				reservationPod := createGpuIdxReservationPod(ptr.To(gpuIdx))
				_, err := kubeclient.CoreV1().Pods(resourceReservationNs).Create(context.TODO(), reservationPod, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				expectTopologyToBeUpdatedWithReservationPod()

				// Test shared gpu pod handling
				pod := createGpuIdxSharedGpuPod(gpuIdx, 0.5)
				_, err = kubeclient.CoreV1().Pods(podNamespace).Create(context.TODO(), pod, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				podGroup := createPodGroup("train")
				_, err = dynamicClient.Resource(schema.GroupVersionResource{Group: "scheduling.run.ai", Version: "v1", Resource: "podgroups"}).Namespace(podNamespace).Create(context.TODO(), podGroup, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				expectTopologyToBeUpdatedWithSharedGpuPod()
			})
		})

		Context("with a runai-gpu-group label", func() {
			It("should update the cluster topology at its reservation pod location", func() {
				gpuGroup := "group1"

				// Test reservation pod handling
				reservationPod := createGpuGroupReservationPod(gpuGroup)
				_, err := kubeclient.CoreV1().Pods(resourceReservationNs).Create(context.TODO(), reservationPod, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				expectTopologyToBeUpdatedWithReservationPod()

				// Test shared gpu pod handling
				pod := createGpuGroupSharedGpuPod(gpuGroup, 0.5)
				_, err = kubeclient.CoreV1().Pods(podNamespace).Create(context.TODO(), pod, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				podGroup := createPodGroup("train")
				_, err = dynamicClient.Resource(schema.GroupVersionResource{Group: "scheduling.run.ai", Version: "v1", Resource: "podgroups"}).Namespace(podNamespace).Create(context.TODO(), podGroup, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())

				expectTopologyToBeUpdatedWithSharedGpuPod()
			})
		})
	})

	When("informed of a GPU node", func() {
		It("should create a new now topology", func() {
			node := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Labels: map[string]string{
						"run.ai/simulated-gpu-node-pool": "default",
					},
				},
			}

			_, err := kubeclient.CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			Eventually(getTopologyNodeFromKube(kubeclient, node.Name)).Should(Not(BeNil()))

			clusterTopology, err := getTopologyFromKube(kubeclient)()
			Expect(err).ToNot(HaveOccurred())
			Expect(clusterTopology).ToNot(BeNil())

			nodeTopology, err := getTopologyNodeFromKube(kubeclient, node.Name)()
			Expect(err).ToNot(HaveOccurred())
			Expect(nodeTopology).ToNot(BeNil())

			Expect(nodeTopology.GpuMemory).To(Equal(clusterTopology.NodePools["default"].GpuMemory))
			Expect(nodeTopology.GpuProduct).To(Equal(clusterTopology.NodePools["default"].GpuProduct))
			Expect(nodeTopology.Gpus).To(HaveLen(clusterTopology.NodePools["default"].GpuCount))
			Expect(nodeTopology.MigStrategy).To(Equal(clusterTopology.MigStrategy))
		})
	})

	When("informed of a node without GPU labels", func() {
		It("should not add the node to the cluster topology", func() {
			node := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
				},
			}

			_, err := kubeclient.CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
			Consistently(getTopologyNodeFromKubeErrorOrNil(kubeclient, node.Name)).Should(MatchError(errors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, topology.GetNodeTopologyCMName(node.Name))))
		})
	})

	When("informed of a node deletion", func() {
		It("should remove the node from the cluster topology", func() {
			node := &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Labels: map[string]string{
						"run.ai/simulated-gpu-node-pool": "default",
					},
				},
			}

			_, err := kubeclient.CoreV1().Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			Eventually(getTopologyNodeFromKubeErrorOrNil(kubeclient, node.Name)).Should(BeNil())

			err = kubeclient.CoreV1().Nodes().Delete(context.TODO(), node.Name, metav1.DeleteOptions{})
			Expect(err).ToNot(HaveOccurred())

			Eventually(getTopologyNodeFromKubeErrorOrNil(kubeclient, node.Name)).Should(Not(BeNil()))
		})
	})
})

func getTopologyFromKube(kubeclient kubernetes.Interface) func() (*topology.ClusterTopology, error) {
	return func() (*topology.ClusterTopology, error) {
		ret, err := topology.GetClusterTopologyFromCM(kubeclient)
		return ret, err
	}
}

func getTopologyNodeFromKube(kubeclient kubernetes.Interface, nodeName string) func() (*topology.NodeTopology, error) {
	return func() (*topology.NodeTopology, error) {
		topology, err := topology.GetNodeTopologyFromCM(kubeclient, nodeName)
		if err != nil {
			return nil, err
		}

		return topology, nil
	}
}

func getTopologyNodeFromKubeErrorOrNil(kubeclient kubernetes.Interface, nodeName string) func() error {
	return func() error {
		_, err := topology.GetNodeTopologyFromCM(kubeclient, nodeName)
		if err != nil {
			return err
		}

		return nil
	}
}

func setupFakes(kubeclient kubernetes.Interface, dynamicClient dynamic.Interface) {
	status_updater.InClusterConfigFn = func() *rest.Config {
		return &rest.Config{}
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
	if err := os.Setenv(constants.EnvTopologyCmName, "fake-cm-name"); err != nil {
		panic(fmt.Sprintf("Failed to set topology CM name: %v", err))
	}
	if err := os.Setenv(constants.EnvTopologyCmNamespace, "fake-cm-namespace"); err != nil {
		panic(fmt.Sprintf("Failed to set topology CM namespace: %v", err))
	}
	if err := os.Setenv(constants.EnvResourceReservationNamespace, "runai-reservation"); err != nil {
		panic(fmt.Sprintf("Failed to set resource reservation namespace: %v", err))
	}
}

func createTopology(gpuCount int64, nodeName string) *topology.NodeTopology {
	gpus := make([]topology.GpuDetails, gpuCount)
	for i := int64(0); i < gpuCount; i++ {
		gpus[i] = topology.GpuDetails{
			ID: fmt.Sprintf("GPU-%s", uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("%s-%d", nodeName, i)))),
			Status: topology.GpuStatus{
				PodGpuUsageStatus: topology.PodGpuUsageStatusMap{},
			},
		}
	}

	return &topology.NodeTopology{
		MigStrategy: "mixed",
		GpuMemory:   11441,
		GpuProduct:  "Tesla-K80",
		Gpus:        gpus,
	}
}

func createDedicatedGpuPod(gpuCount int64, phase v1.PodPhase, conditions []v1.PodCondition) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
			UID:       podUID,
			Annotations: map[string]string{
				constants.AnnotationPodGroupName: podGroupName,
			},
		},
		Spec: v1.PodSpec{
			NodeName: node,
			Containers: []v1.Container{
				{
					Name: containerName,
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							constants.GpuResourceName: *resource.NewQuantity(gpuCount, resource.DecimalSI),
						},
					},
				},
			},
		},
		Status: v1.PodStatus{
			Phase:      phase,
			Conditions: conditions,
		},
	}
}

func createGpuIdxSharedGpuPod(gpuIdx int, gpuFraction float64) *v1.Pod {
	pod := createBaseSharedGpuPod(gpuFraction)

	pod.Annotations[constants.AnnotationGpuIdx] = fmt.Sprintf("%d", gpuIdx)

	return pod
}

func createGpuGroupSharedGpuPod(gpuGroup string, gpuFraction float64) *v1.Pod {
	pod := createBaseSharedGpuPod(gpuFraction)
	pod.Labels[constants.LabelGpuGroup] = gpuGroup
	return pod
}

func createBaseSharedGpuPod(gpuFraction float64) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: podNamespace,
			UID:       podUID,
			Annotations: map[string]string{
				constants.AnnotationGpuFraction:  fmt.Sprintf("%f", gpuFraction),
				constants.AnnotationPodGroupName: podGroupName,
			},
			Labels: map[string]string{},
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
			Phase: v1.PodRunning,
			Conditions: []v1.PodCondition{
				{
					Type:   v1.PodScheduled,
					Status: v1.ConditionTrue,
				},
			},
		},
	}
}

func createGpuIdxReservationPod(gpuIdx *int) *v1.Pod {
	pod := createBaseReservationPod()

	if gpuIdx != nil {
		pod.Annotations[constants.AnnotationReservationPodGpuIdx] = strconv.Itoa(*gpuIdx)
	}
	return pod
}

func createGpuGroupReservationPod(gpuGroup string) *v1.Pod {
	pod := createBaseReservationPod()
	pod.Labels[constants.LabelGpuGroup] = gpuGroup
	return pod
}

func createBaseReservationPod() *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        reservationPodName,
			Namespace:   resourceReservationNs,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Spec: v1.PodSpec{
			NodeName: node,
			Containers: []v1.Container{
				{
					Name: reservationPodContainerName,
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							constants.GpuResourceName: *resource.NewQuantity(1, resource.DecimalSI),
						},
					},
				},
			},
		},
		Status: v1.PodStatus{
			Phase:      v1.PodRunning,
			Conditions: []v1.PodCondition{{Type: v1.PodScheduled, Status: v1.ConditionTrue}},
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

func getExpectedUtilization(workloadType string, podPhase v1.PodPhase) topology.Range {
	if podPhase != v1.PodRunning {
		return topology.Range{
			Min: 0,
			Max: 0,
		}
	}

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

func isConditionTrue(conditions []v1.PodCondition, conditionType v1.PodConditionType) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType && condition.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}
