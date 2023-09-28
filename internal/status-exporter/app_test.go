package status_exporter_test

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/run-ai/fake-gpu-operator/internal/common/app"
	"github.com/run-ai/fake-gpu-operator/internal/common/kubeclient"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	status_exporter "github.com/run-ai/fake-gpu-operator/internal/status-exporter"
)

type testCase struct {
	nodeTopologies  map[string]*topology.NodeTopology
	expectedLabels  map[string]string
	expectedMetrics []*dto.MetricFamily
}

const (
	topologyCmName      = "fake-cm-name"
	topologyCmNamespace = "fake-cm-namespace"
	podNamespace        = "fake-pod-namespace"
	podName             = "fake-pod-name"
	containerName       = "fake-container-name"
	nodeName            = "fake-node"
)

func TestStatusExporter(t *testing.T) {
	format.MaxLength = 10000
	RegisterFailHandler(Fail)
	RunSpecs(t, "StatusExporter Suite")
}

// Test plan:
// For topology input, export the following:
// - node labels
// - node metrics
var _ = Describe("StatusExporter", func() {
	var (
		clientset kubernetes.Interface
	)

	fakeNode := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   nodeName,
			Labels: map[string]string{},
		},
	}
	clientset = fake.NewSimpleClientset(fakeNode)
	setupConfig()

	exporter := &status_exporter.StatusExporterApp{
		Kubeclient: &kubeclient.KubeClient{
			ClientSet: clientset,
		},
	}
	appRunner := app.NewAppRunner(exporter)
	go appRunner.Run()
	// Wait for the status exporter to initialize
	time.Sleep(1000 * time.Millisecond)

	initialTopology := createNodeTopology()
	cm, err := topology.ToNodeTopologyCM(initialTopology, nodeName)
	Expect(err).To(Not(HaveOccurred()))
	_, err = clientset.CoreV1().ConfigMaps(topologyCmNamespace).Create(context.TODO(), cm, metav1.CreateOptions{})
	Expect(err).To(Not(HaveOccurred()))

	cases := getTestCases()

	for caseName, caseDetails := range cases {
		caseName := caseName
		caseDetails := caseDetails

		It(caseName, func() {
			cm, err := topology.ToNodeTopologyCM(caseDetails.nodeTopologies[nodeName], nodeName)
			Expect(err).ToNot(HaveOccurred())

			_, err = clientset.CoreV1().ConfigMaps(topologyCmNamespace).Update(context.TODO(), cm, metav1.UpdateOptions{})
			Expect(err).ToNot(HaveOccurred())

			Eventually(getNodeLabelsFromKube(clientset)).WithTimeout(19 * time.Second).Should(Equal(caseDetails.expectedLabels))
			Eventually(getNodeMetrics()).Should(ContainElements(caseDetails.expectedMetrics))
		})
	}
})

func setupConfig() {
	setupEnvs()
}

func setupEnvs() {
	os.Setenv("TOPOLOGY_CM_NAME", topologyCmName)
	os.Setenv("TOPOLOGY_CM_NAMESPACE", topologyCmNamespace)
	os.Setenv("NODE_NAME", nodeName)
	os.Setenv("KUBERNETES_SERVICE_HOST", "fake-k8s-service-host")
	os.Setenv("KUBERNETES_SERVICE_PORT", "fake-k8s-service-port")
}

func getNodeLabelsFromKube(kubeclient kubernetes.Interface) func() (map[string]string, error) {
	return func() (map[string]string, error) {
		node, err := kubeclient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}

		return node.Labels, nil
	}
}

func getNodeMetrics() func() ([]*dto.MetricFamily, error) {
	return func() ([]*dto.MetricFamily, error) {
		return prometheus.DefaultGatherer.Gather()
	}
}

func buildMetricLabels(labels map[string]string) []*dto.LabelPair {
	// Sort the labels to make the test deterministic
	var keys []string
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var metricLabels []*dto.LabelPair
	for _, k := range keys {
		metricLabels = append(metricLabels, &dto.LabelPair{
			Name:  createPtr(k),
			Value: createPtr(labels[k]),
		})
	}

	return metricLabels
}

func createPtr[T any](val T) *T {
	return &val
}

func getTestCases() map[string]testCase {
	return map[string]testCase{
		"Single GPU": {
			nodeTopologies: map[string]*topology.NodeTopology{
				nodeName: {
					MigStrategy: "mixed",
					GpuMemory:   20000,
					GpuProduct:  "Tesla P100",
					Gpus: []topology.GpuDetails{
						{
							ID: "fake-gpu-id-1",
							Status: topology.GpuStatus{
								AllocatedBy: topology.ContainerDetails{
									Namespace: podNamespace,
									Pod:       podName,
									Container: containerName,
								},
								PodGpuUsageStatus: topology.PodGpuUsageStatusMap{
									podName: topology.GpuUsageStatus{
										Utilization: topology.Range{
											Min: 80,
											Max: 80,
										},
										FbUsed: 20000,
									},
								},
							},
						},
					},
				},
			},
			expectedLabels: map[string]string{
				"nvidia.com/gpu.present":  "true",
				"nvidia.com/gpu.memory":   "20000",
				"nvidia.com/gpu.count":    "1",
				"nvidia.com/mig.strategy": "mixed",
				"nvidia.com/gpu.product":  "Tesla P100",
			},
			expectedMetrics: []*dto.MetricFamily{
				{
					Name: createPtr("DCGM_FI_DEV_GPU_UTIL"),
					Help: createPtr("GPU Utilization"),
					Type: createPtr(dto.MetricType_GAUGE),
					Metric: []*dto.Metric{
						{
							Label: buildMetricLabels(map[string]string{
								"pod":       podName,
								"namespace": podNamespace,
								"device":    "nvidia0",
								"gpu":       "0",
								"modelName": "Tesla P100",
								"Hostname":  "nvidia-dcgm-exporter-ce8600",
								"UUID":      "fake-gpu-id-1",
								"container": containerName,
							}),
							Gauge: &dto.Gauge{
								Value: createPtr(float64(80)),
							},
						},
					},
				},
				{
					Name: createPtr("DCGM_FI_DEV_FB_FREE"),
					Help: createPtr("GPU Framebuffer Free"),
					Type: createPtr(dto.MetricType_GAUGE),
					Metric: []*dto.Metric{
						{
							Label: buildMetricLabels(map[string]string{
								"Hostname":  "nvidia-dcgm-exporter-ce8600",
								"UUID":      "fake-gpu-id-1",
								"pod":       podName,
								"namespace": podNamespace,
								"device":    "nvidia0",
								"gpu":       "0",
								"modelName": "Tesla P100",
								"container": containerName,
							}),
							Gauge: &dto.Gauge{
								Value: createPtr(float64(0)),
							},
						},
					},
				},
				{
					Name: createPtr("DCGM_FI_DEV_FB_USED"),
					Help: createPtr("GPU Framebuffer Used"),
					Type: createPtr(dto.MetricType_GAUGE),
					Metric: []*dto.Metric{
						{
							Label: buildMetricLabels(map[string]string{
								"Hostname":  "nvidia-dcgm-exporter-ce8600",
								"UUID":      "fake-gpu-id-1",
								"pod":       podName,
								"namespace": podNamespace,
								"device":    "nvidia0",
								"gpu":       "0",
								"modelName": "Tesla P100",
								"container": containerName,
							}),
							Gauge: &dto.Gauge{
								Value: createPtr(float64(20000)),
							},
						},
					},
				},
			},
		},
		"Multiple GPUs": {
			nodeTopologies: map[string]*topology.NodeTopology{
				nodeName: {
					MigStrategy: "mixed",
					GpuMemory:   20000,
					GpuProduct:  "Tesla P100",
					Gpus: []topology.GpuDetails{
						{
							ID: "fake-gpu-id-1",
							Status: topology.GpuStatus{
								AllocatedBy: topology.ContainerDetails{
									Namespace: podNamespace,
									Pod:       podName,
									Container: containerName,
								},
								PodGpuUsageStatus: topology.PodGpuUsageStatusMap{
									podName: topology.GpuUsageStatus{
										Utilization: topology.Range{
											Min: 100,
											Max: 100,
										},
										FbUsed: 20000,
									},
								},
							},
						},
						{
							ID: "fake-gpu-id-2",
							Status: topology.GpuStatus{
								AllocatedBy: topology.ContainerDetails{
									Namespace: podNamespace,
									Pod:       podName,
									Container: containerName,
								},
								PodGpuUsageStatus: topology.PodGpuUsageStatusMap{
									podName: topology.GpuUsageStatus{
										Utilization: topology.Range{
											Min: 100,
											Max: 100,
										},
										FbUsed: 20000,
									},
								},
							},
						},
					},
				},
			},
			expectedLabels: map[string]string{
				"nvidia.com/gpu.present":  "true",
				"nvidia.com/gpu.memory":   "20000",
				"nvidia.com/gpu.count":    "2",
				"nvidia.com/mig.strategy": "mixed",
				"nvidia.com/gpu.product":  "Tesla P100",
			},
			expectedMetrics: []*dto.MetricFamily{
				{
					Name: createPtr("DCGM_FI_DEV_GPU_UTIL"),
					Help: createPtr("GPU Utilization"),
					Type: createPtr(dto.MetricType_GAUGE),
					Metric: []*dto.Metric{
						{
							Label: buildMetricLabels(map[string]string{
								"Hostname":  "nvidia-dcgm-exporter-ce8600",
								"UUID":      "fake-gpu-id-1",
								"pod":       podName,
								"namespace": podNamespace,
								"device":    "nvidia0",
								"gpu":       "0",
								"modelName": "Tesla P100",
								"container": containerName,
							}),
							Gauge: &dto.Gauge{
								Value: createPtr(float64(100)),
							},
						},
						{
							Label: buildMetricLabels(map[string]string{
								"Hostname":  "nvidia-dcgm-exporter-ce8600",
								"UUID":      "fake-gpu-id-2",
								"pod":       podName,
								"namespace": podNamespace,
								"device":    "nvidia1",
								"gpu":       "1",
								"modelName": "Tesla P100",
								"container": containerName,
							}),
							Gauge: &dto.Gauge{
								Value: createPtr(float64(100)),
							},
						},
					},
				},
				{
					Name: createPtr("DCGM_FI_DEV_FB_USED"),
					Help: createPtr("GPU Framebuffer Used"),
					Type: createPtr(dto.MetricType_GAUGE),
					Metric: []*dto.Metric{
						{
							Label: buildMetricLabels(map[string]string{
								"Hostname":  "nvidia-dcgm-exporter-ce8600",
								"UUID":      "fake-gpu-id-1",
								"pod":       podName,
								"namespace": podNamespace,
								"device":    "nvidia0",
								"gpu":       "0",
								"modelName": "Tesla P100",
								"container": containerName,
							}),
							Gauge: &dto.Gauge{
								Value: createPtr(float64(20000)),
							},
						},
						{
							Label: buildMetricLabels(map[string]string{
								"Hostname":  "nvidia-dcgm-exporter-ce8600",
								"UUID":      "fake-gpu-id-2",
								"pod":       podName,
								"namespace": podNamespace,
								"device":    "nvidia1",
								"gpu":       "1",
								"modelName": "Tesla P100",
								"container": containerName,
							}),
							Gauge: &dto.Gauge{
								Value: createPtr(float64(20000)),
							},
						},
					},
				},
			},
		},
	}
}

func createNodeTopology() *topology.NodeTopology {
	return &topology.NodeTopology{
		MigStrategy: "mixed",
		GpuMemory:   20000,
		GpuProduct:  "Tesla P100",
		Gpus: []topology.GpuDetails{
			{
				ID: "fake-gpu-id-1",
				Status: topology.GpuStatus{
					AllocatedBy: topology.ContainerDetails{
						Namespace: podNamespace,
						Pod:       podName,
						Container: containerName,
					},
					PodGpuUsageStatus: topology.PodGpuUsageStatusMap{
						podName: topology.GpuUsageStatus{
							Utilization: topology.Range{
								Min: 100,
								Max: 100,
							},
							FbUsed: 20000,
						},
					},
				},
			},
		},
	}
}
