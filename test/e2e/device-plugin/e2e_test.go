package deviceplugin_e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

const (
	// Namespace where helm installs everything.
	releaseNs = "gpu-operator"

	// Node name matches kind-cluster-config.yaml (kind names workers
	// "<cluster>-worker") and setup.sh's labelling.
	workerNode = "device-plugin-cluster-worker"

	// gpuCount matches values.yaml's topology.nodePools.mig-pool.gpuCount.
	gpuCount = 2

	// MIG profile under test (valid for A100-40GB per migfaker's mapping).
	migProfile  = "1g.5gb"
	migResource = "nvidia.com/mig-" + migProfile

	// Number of 1g.5gb slices we ask mig-faker to create on GPU 0.
	migSliceCount = 7

	// A second profile used to exercise reconfiguration (also valid for
	// A100-40GB). Switching to it must replace the 1g.5gb resources.
	migProfile2  = "2g.10gb"
	migResource2 = "nvidia.com/mig-" + migProfile2
	migSliceCount2 = 3

	gpuResource = "nvidia.com/gpu"

	migConfigAnnotation = "run.ai/mig.config"
	migStateLabel       = "nvidia.com/mig.config.state"

	eventuallyTimeout = 3 * time.Minute
	pollInterval      = 3 * time.Second
)

var (
	kubeClient kubernetes.Interface
	restConfig *rest.Config
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Device-Plugin / MIG E2E Suite")
}

var _ = BeforeSuite(func() {
	var err error

	restConfig, err = getKubeConfig()
	Expect(err).NotTo(HaveOccurred())

	kubeClient, err = kubernetes.NewForConfig(restConfig)
	Expect(err).NotTo(HaveOccurred())

	// Sanity: cluster reachable and the MIG worker exists.
	_, err = kubeClient.CoreV1().Nodes().Get(context.Background(), workerNode, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred(), "MIG worker node %q must exist (set up by setup.sh)", workerNode)
})

// The specs run in order: verify the plain-GPU baseline, apply a MIG config,
// then verify mig-faker + device-plugin turn it into nvidia.com/mig-* resources.
var _ = Describe("mig-faker advertises MIG resources via the device-plugin", Ordered, func() {
	AfterAll(func() {
		// Best-effort cleanup so a re-run with SKIP_TEARDOWN=true starts clean:
		// removing the annotation makes mig-faker clear the MIG state and the
		// device-plugin returns to advertising whole GPUs.
		removeNodeAnnotation(workerNode, migConfigAnnotation)
	})

	It("advertises whole GPUs and no MIG resources before any MIG config", func() {
		Eventually(func() int64 {
			return allocatableQuantity(workerNode, gpuResource)
		}).WithTimeout(eventuallyTimeout).WithPolling(pollInterval).
			Should(Equal(int64(gpuCount)), "node should advertise %d whole GPUs initially", gpuCount)

		Expect(allocatableHas(workerNode, migResource)).To(BeFalse(),
			"no nvidia.com/mig-* resources should exist before a MIG config is applied")
	})

	It("accepts a run.ai/mig.config annotation and reports mig.config.state=success", func() {
		setNodeAnnotation(workerNode, migConfigAnnotation, buildMigConfig(0, migProfile, migSliceCount))

		Eventually(func() string {
			node, err := kubeClient.CoreV1().Nodes().Get(context.Background(), workerNode, metav1.GetOptions{})
			if err != nil {
				return ""
			}
			return node.Labels[migStateLabel]
		}).WithTimeout(eventuallyTimeout).WithPolling(pollInterval).
			Should(Equal("success"), "mig-faker should set %s=success", migStateLabel)
	})

	It("writes MigInstances into the per-node topology ConfigMap", func() {
		Eventually(func(g Gomega) {
			nodeTopology := getNodeTopology(g, workerNode)
			g.Expect(nodeTopology.Gpus).To(HaveLen(gpuCount))

			// GPU 0 was selected for MIG.
			g.Expect(nodeTopology.Gpus[0].MigEnabled).To(BeTrue(), "GPU 0 should be MIG-enabled")
			g.Expect(nodeTopology.Gpus[0].MigInstances).To(HaveLen(migSliceCount))
			for _, instance := range nodeTopology.Gpus[0].MigInstances {
				g.Expect(instance.Profile).To(Equal(migProfile))
				g.Expect(instance.UUID).To(HavePrefix("MIG-"))
			}

			// GPU 1 was not selected, so it stays a whole GPU.
			g.Expect(nodeTopology.Gpus[1].MigEnabled).To(BeFalse(), "GPU 1 should remain a whole GPU")
		}).WithTimeout(eventuallyTimeout).WithPolling(pollInterval).Should(Succeed())
	})

	It("advertises nvidia.com/mig-<profile> in node allocatable and drops the MIG'd GPU", func() {
		Eventually(func() int64 {
			return allocatableQuantity(workerNode, migResource)
		}).WithTimeout(eventuallyTimeout).WithPolling(pollInterval).
			Should(Equal(int64(migSliceCount)), "node should advertise %d %s", migSliceCount, migResource)

		// One of the two GPUs became MIG, so a single whole GPU remains.
		Eventually(func() int64 {
			return allocatableQuantity(workerNode, gpuResource)
		}).WithTimeout(eventuallyTimeout).WithPolling(pollInterval).
			Should(Equal(int64(gpuCount-1)), "the MIG-enabled GPU should no longer count toward %s", gpuResource)
	})

	It("schedules a pod that requests the MIG resource", func() {
		const (
			ns      = "mig-sched-test"
			podName = "mig-consumer"
		)
		createNamespace(ns)
		DeferCleanup(func() { deleteNamespace(ns) })

		_, err := kubeClient.CoreV1().Pods(ns).Create(context.Background(), &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: podName, Namespace: ns},
			Spec: corev1.PodSpec{
				RestartPolicy: corev1.RestartPolicyNever,
				Containers: []corev1.Container{{
					Name:    "ctr0",
					Image:   "ubuntu:24.04",
					Command: []string{"bash", "-c", "trap 'exit 0' TERM; sleep 9999 & wait"},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							migResource: resource.MustParse("1"),
						},
					},
				}},
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() corev1.PodPhase {
			pod, err := kubeClient.CoreV1().Pods(ns).Get(context.Background(), podName, metav1.GetOptions{})
			if err != nil {
				return ""
			}
			return pod.Status.Phase
		}).WithTimeout(eventuallyTimeout).WithPolling(pollInterval).
			Should(Equal(corev1.PodRunning), "pod requesting %s should be scheduled and run", migResource)
	})

	It("replaces the MIG resources when the config changes to a different profile", func() {
		// Re-annotate GPU 0 with a different profile. mig-faker must rewrite the
		// topology CM and restart the device-plugin, whose CleanupStaleSockets
		// removes the old 1g.5gb socket so it no longer lingers in allocatable.
		setNodeAnnotation(workerNode, migConfigAnnotation, buildMigConfig(0, migProfile2, migSliceCount2))

		Eventually(func(g Gomega) {
			nodeTopology := getNodeTopology(g, workerNode)
			g.Expect(nodeTopology.Gpus[0].MigInstances).To(HaveLen(migSliceCount2))
			g.Expect(nodeTopology.Gpus[0].MigInstances[0].Profile).To(Equal(migProfile2))
		}).WithTimeout(eventuallyTimeout).WithPolling(pollInterval).Should(Succeed())

		// The new profile shows up...
		Eventually(func() int64 {
			return allocatableQuantity(workerNode, migResource2)
		}).WithTimeout(eventuallyTimeout).WithPolling(pollInterval).
			Should(Equal(int64(migSliceCount2)), "node should advertise %d %s after reconfiguration", migSliceCount2, migResource2)

		// ...and the old one is gone (0 or absent), proving stale sockets are cleaned.
		Eventually(func() int64 {
			return allocatableQuantity(workerNode, migResource)
		}).WithTimeout(eventuallyTimeout).WithPolling(pollInterval).
			Should(BeZero(), "the previous %s resource should no longer be advertised", migResource)
	})
})

// migConfigTemplate renders a run.ai/mig.config YAML matching the
// migfaker.AnnotationMigConfig shape: a single selected GPU with N slices of a
// profile, positions 0..N-1.
var migConfigTemplate = template.Must(template.New("migConfig").Parse(
	`version: v1
mig-configs:
  selected:
  - devices: ["{{ .GpuIndex }}"]
    mig-enabled: true
    mig-devices:
{{- range .Positions }}
    - name: {{ $.Profile }}
      position: {{ . }}
      size: 1
{{- end }}
`))

// buildMigConfig enables MIG on a single GPU index with `count` slices of the
// given profile.
func buildMigConfig(gpuIndex int, profile string, count int) string {
	positions := make([]int, count)
	for i := range positions {
		positions[i] = i
	}

	var b strings.Builder
	err := migConfigTemplate.Execute(&b, struct {
		GpuIndex  int
		Profile   string
		Positions []int
	}{GpuIndex: gpuIndex, Profile: profile, Positions: positions})
	Expect(err).NotTo(HaveOccurred())
	return b.String()
}

func getNodeTopology(g Gomega, nodeName string) *topology.NodeTopology {
	cmList, err := kubeClient.CoreV1().ConfigMaps(releaseNs).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("node-name=%s", nodeName),
	})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(cmList.Items).To(HaveLen(1), "expected exactly one topology ConfigMap for %s", nodeName)

	nodeTopology, err := topology.FromNodeTopologyCM(&cmList.Items[0])
	g.Expect(err).NotTo(HaveOccurred())
	return nodeTopology
}

func allocatableQuantity(nodeName, resourceName string) int64 {
	node, err := kubeClient.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
	if err != nil {
		return -1
	}
	qty, ok := node.Status.Allocatable[corev1.ResourceName(resourceName)]
	if !ok {
		return 0
	}
	return qty.Value()
}

func allocatableHas(nodeName, resourceName string) bool {
	node, err := kubeClient.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
	if err != nil {
		return false
	}
	_, ok := node.Status.Allocatable[corev1.ResourceName(resourceName)]
	return ok
}

func setNodeAnnotation(nodeName, key, value string) {
	patch := map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]string{key: value},
		},
	}
	patchBytes, err := json.Marshal(patch)
	Expect(err).NotTo(HaveOccurred())
	_, err = kubeClient.CoreV1().Nodes().Patch(context.Background(), nodeName, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func removeNodeAnnotation(nodeName, key string) {
	// A null value in a merge patch deletes the annotation key.
	patch := map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]any{key: nil},
		},
	}
	patchBytes, err := json.Marshal(patch)
	Expect(err).NotTo(HaveOccurred())
	_, _ = kubeClient.CoreV1().Nodes().Patch(context.Background(), nodeName, types.MergePatchType, patchBytes, metav1.PatchOptions{})
}

func createNamespace(name string) {
	_, err := kubeClient.CoreV1().Namespaces().Create(context.Background(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}, metav1.CreateOptions{})
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		Expect(err).NotTo(HaveOccurred())
	}
}

func deleteNamespace(name string) {
	_ = kubeClient.CoreV1().Namespaces().Delete(context.Background(), name, metav1.DeleteOptions{})
}

func getKubeConfig() (*rest.Config, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		kubeconfig = filepath.Join(home, ".kube", "config")
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig from %q: %w", kubeconfig, err)
	}
	return cfg, nil
}
