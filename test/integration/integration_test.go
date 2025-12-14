package integration_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	testTimeout      = 5 * time.Minute
	podReadyTimeout  = 2 * time.Minute
	namespaceTimeout = 30 * time.Second
)

var (
	kubeClient kubernetes.Interface
	restConfig *rest.Config
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	var err error

	// Get Kubernetes config
	restConfig, err = getKubeConfig()
	Expect(err).NotTo(HaveOccurred())

	// Create Kubernetes client
	kubeClient, err = kubernetes.NewForConfig(restConfig)
	Expect(err).NotTo(HaveOccurred())

	// Verify cluster is accessible
	_, err = kubeClient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	// Verify DRA plugin is running
	Eventually(func() error {
		pods, err := kubeClient.CoreV1().Pods("gpu-operator").List(context.Background(), metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/component=kubeletplugin",
		})
		if err != nil {
			return err
		}
		if len(pods.Items) == 0 {
			return fmt.Errorf("no DRA plugin pods found")
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				return fmt.Errorf("pod %s is not running: %s", pod.Name, pod.Status.Phase)
			}
		}
		return nil
	}).WithTimeout(testTimeout).Should(Succeed())
})

// resourceClaimInfo tracks ResourceClaim information for cleanup
type resourceClaimInfo struct {
	namespace string
	name      string
}

var _ = Describe("DRA Plugin Integration Tests", func() {
	var testNamespaces []string
	var testResourceClaims []resourceClaimInfo

	AfterEach(func() {
		// Cleanup ResourceClaims first to free up GPUs
		for _, claim := range testResourceClaims {
			deleteResourceClaim(claim.namespace, claim.name)
		}
		testResourceClaims = nil

		// Cleanup namespaces (this will also delete pods)
		for _, ns := range testNamespaces {
			deleteNamespace(ns)
		}
		testNamespaces = nil
	})

	Describe("Basic GPU Allocation", func() {
		It("should allocate a GPU to a pod", func() {
			manifestPath := filepath.Join("manifests", "basic-pod.yaml")
			namespace := "gpu-test-basic"
			podName := "pod0"

			setupTest(manifestPath, namespace, &testNamespaces)
			waitAndTrackResourceClaims(namespace, []string{podName}, &testResourceClaims)
			waitForPodReady(namespace, podName, podReadyTimeout)

			logs := getPodLogs(namespace, podName, "ctr0")
			envVars := extractGPUEnvVars(logs)
			Expect(envVars).To(HaveLen(1), "Expected 1 GPU device")

			gpuDevice := envVars[0]
			Expect(gpuDevice).To(MatchRegexp(`^gpu-[a-z0-9-]+$`), "GPU device should match pattern")

			deviceID := extractDeviceID(gpuDevice)
			verifyEnvVars(logs, gpuDevice, map[string]types.GomegaMatcher{
				fmt.Sprintf("GPU_DEVICE_%s_RESOURCE_CLAIM", deviceID): MatchRegexp(`^[a-z0-9-]+$`),
			})
			verifyNvidiaSmiBinary(namespace, podName, "ctr0")
		})
	})

	Describe("Multiple Pods", func() {
		It("should allocate distinct GPUs to multiple pods", func() {
			manifestPath := filepath.Join("manifests", "multi-pod.yaml")
			namespace := "gpu-test-multi"
			podNames := []string{"pod0", "pod1"}

			setupTest(manifestPath, namespace, &testNamespaces)
			waitAndTrackResourceClaims(namespace, podNames, &testResourceClaims)

			for _, podName := range podNames {
				waitForPodReady(namespace, podName, podReadyTimeout)
			}

			pod0Logs := getPodLogs(namespace, "pod0", "ctr0")
			pod1Logs := getPodLogs(namespace, "pod1", "ctr0")

			pod0GPUs := extractGPUEnvVars(pod0Logs)
			pod1GPUs := extractGPUEnvVars(pod1Logs)

			Expect(pod0GPUs).To(HaveLen(1), "Pod0 should have 1 GPU")
			Expect(pod1GPUs).To(HaveLen(1), "Pod1 should have 1 GPU")
			Expect(pod0GPUs[0]).NotTo(Equal(pod1GPUs[0]), "Pods should have distinct GPUs")
		})
	})

	Describe("TimeSlicing Sharing Strategy", func() {
		It("should share GPU between containers using TimeSlicing", func() {
			manifestPath := filepath.Join("manifests", "timeslicing-pod.yaml")
			namespace := "gpu-test-timeslicing"
			podName := "pod0"

			setupTest(manifestPath, namespace, &testNamespaces)
			waitAndTrackResourceClaims(namespace, []string{podName}, &testResourceClaims)
			waitForPodReady(namespace, podName, podReadyTimeout)

			ctr0Logs := getPodLogs(namespace, podName, "ctr0")
			ctr1Logs := getPodLogs(namespace, podName, "ctr1")

			ctr0GPUs := extractGPUEnvVars(ctr0Logs)
			ctr1GPUs := extractGPUEnvVars(ctr1Logs)

			Expect(ctr0GPUs).To(HaveLen(1), "Container 0 should have 1 GPU")
			Expect(ctr1GPUs).To(HaveLen(1), "Container 1 should have 1 GPU")
			Expect(ctr0GPUs[0]).To(Equal(ctr1GPUs[0]), "Containers should share the same GPU")

			deviceID := extractDeviceID(ctr0GPUs[0])
			verifyEnvVars(ctr0Logs, ctr0GPUs[0], map[string]types.GomegaMatcher{
				fmt.Sprintf("GPU_DEVICE_%s_SHARING_STRATEGY", deviceID):   Equal("TimeSlicing"),
				fmt.Sprintf("GPU_DEVICE_%s_TIMESLICE_INTERVAL", deviceID): Equal("Default"),
			})
		})
	})

	Describe("SpacePartitioning Sharing Strategy", func() {
		It("should share GPU between containers using SpacePartitioning", func() {
			manifestPath := filepath.Join("manifests", "spacepartitioning-pod.yaml")
			namespace := "gpu-test-spacepartitioning"
			podName := "pod0"

			setupTest(manifestPath, namespace, &testNamespaces)
			waitAndTrackResourceClaims(namespace, []string{podName}, &testResourceClaims)
			waitForPodReady(namespace, podName, podReadyTimeout)

			ctr0Logs := getPodLogs(namespace, podName, "ctr0")
			ctr1Logs := getPodLogs(namespace, podName, "ctr1")

			ctr0GPUs := extractGPUEnvVars(ctr0Logs)
			ctr1GPUs := extractGPUEnvVars(ctr1Logs)

			Expect(ctr0GPUs).To(HaveLen(1), "Container 0 should have 1 GPU")
			Expect(ctr1GPUs).To(HaveLen(1), "Container 1 should have 1 GPU")
			Expect(ctr0GPUs[0]).To(Equal(ctr1GPUs[0]), "Containers should share the same GPU")

			deviceID := extractDeviceID(ctr0GPUs[0])
			verifyEnvVars(ctr0Logs, ctr0GPUs[0], map[string]types.GomegaMatcher{
				fmt.Sprintf("GPU_DEVICE_%s_SHARING_STRATEGY", deviceID): Equal("SpacePartitioning"),
				fmt.Sprintf("GPU_DEVICE_%s_PARTITION_COUNT", deviceID):  Equal("10"),
			})
		})
	})

	Describe("CDI Environment Variables", func() {
		It("should inject all required CDI environment variables", func() {
			manifestPath := filepath.Join("manifests", "basic-pod.yaml")
			namespace := "gpu-test-basic"
			podName := "pod0"

			setupTest(manifestPath, namespace, &testNamespaces)
			waitAndTrackResourceClaims(namespace, []string{podName}, &testResourceClaims)
			waitForPodReady(namespace, podName, podReadyTimeout)

			logs := getPodLogs(namespace, podName, "ctr0")
			envVars := extractGPUEnvVars(logs)
			Expect(envVars).To(HaveLen(1), "Expected 1 GPU device")
			gpuDevice := envVars[0]
			deviceID := extractDeviceID(gpuDevice)

			verifyEnvVars(logs, gpuDevice, map[string]types.GomegaMatcher{
				fmt.Sprintf("GPU_DEVICE_%s_RESOURCE_CLAIM", deviceID): MatchRegexp(`^[a-z0-9-]+$`),
			})
		})
	})

	Describe("nvidia-smi Binary", func() {
		It("should have nvidia-smi binary available and working", func() {
			manifestPath := filepath.Join("manifests", "basic-pod.yaml")
			namespace := "gpu-test-basic"
			podName := "pod0"

			setupTest(manifestPath, namespace, &testNamespaces)
			waitAndTrackResourceClaims(namespace, []string{podName}, &testResourceClaims)
			waitForPodReady(namespace, podName, podReadyTimeout)

			verifyNvidiaSmiBinary(namespace, podName, "ctr0")
		})
	})

	Describe("nvidia-smi Utilization", func() {
		It("should report GPU utilization from topology server", func() {
			manifestPath := filepath.Join("manifests", "utilization-range-pod.yaml")
			namespace := "gpu-test-util"
			podName := "pod0"

			setupTest(manifestPath, namespace, &testNamespaces)
			waitAndTrackResourceClaims(namespace, []string{podName}, &testResourceClaims)
			waitForPodReady(namespace, podName, podReadyTimeout)

			// Run nvidia-smi and verify it works
			output := runNvidiaSmi(namespace, podName, "ctr0")
			Expect(output).To(ContainSubstring("NVIDIA-SMI"), "nvidia-smi output should contain NVIDIA-SMI header")
			Expect(output).To(ContainSubstring("GPU"), "nvidia-smi output should contain GPU information")

			// Verify GPU utilization is shown (can be 0-100%)
			Expect(output).To(MatchRegexp(`\d+%`), "nvidia-smi output should show GPU utilization percentage")
		})

		It("should show varying utilization values on repeated runs", func() {
			manifestPath := filepath.Join("manifests", "utilization-range-pod.yaml")
			namespace := "gpu-test-util-vary"
			podName := "pod0"

			// Apply manifest with different namespace
			applyManifestWithNamespace(manifestPath, namespace)
			testNamespaces = append(testNamespaces, namespace)
			waitAndTrackResourceClaims(namespace, []string{podName}, &testResourceClaims)
			waitForPodReady(namespace, podName, podReadyTimeout)

			// Run nvidia-smi multiple times and collect utilization values
			var utilizations []int
			for i := 0; i < 5; i++ {
				util := getNvidiaSmiUtilization(namespace, podName, "ctr0")
				utilizations = append(utilizations, util)
				time.Sleep(100 * time.Millisecond)
			}

			// Verify that utilization values are within the expected range (50-100 from annotation)
			for _, util := range utilizations {
				Expect(util).To(BeNumerically(">=", 0), "Utilization should be >= 0%%")
				Expect(util).To(BeNumerically("<=", 100), "Utilization should be <= 100%%")
			}
		})
	})

	Describe("Prometheus Metrics", func() {
		It("should expose GPU metrics via the status-exporter service", func() {
			// The status-exporter exposes metrics on port 9400 via the nvidia-dcgm-exporter service
			// We use a dedicated manifest to avoid namespace conflicts with other tests
			manifestPath := filepath.Join("manifests", "prometheus-test-pod.yaml")
			namespace := "gpu-test-prometheus"
			podName := "pod0"

			setupTest(manifestPath, namespace, &testNamespaces)
			waitAndTrackResourceClaims(namespace, []string{podName}, &testResourceClaims)
			waitForPodReady(namespace, podName, podReadyTimeout)

			// Give the status-exporter time to export metrics
			time.Sleep(15 * time.Second)

			// Query the metrics endpoint via kubectl port-forward
			metrics := getPrometheusMetrics()

			// Verify expected metrics are present
			Expect(metrics).To(ContainSubstring("DCGM_FI_DEV_GPU_UTIL"),
				"Metrics should contain GPU utilization metric")
			Expect(metrics).To(ContainSubstring("DCGM_FI_DEV_FB_USED"),
				"Metrics should contain GPU framebuffer used metric")
			Expect(metrics).To(ContainSubstring("DCGM_FI_DEV_FB_FREE"),
				"Metrics should contain GPU framebuffer free metric")

			// Verify metrics have the expected labels
			Expect(metrics).To(MatchRegexp(`DCGM_FI_DEV_GPU_UTIL\{.*UUID="GPU-[a-zA-Z0-9-]+"`),
				"GPU utilization metric should have UUID label")
			Expect(metrics).To(MatchRegexp(`DCGM_FI_DEV_GPU_UTIL\{.*modelName="NVIDIA-A100-SXM4-40GB"`),
				"GPU utilization metric should have modelName label")

			// Verify the allocated GPU shows pod information in labels
			Expect(metrics).To(MatchRegexp(`DCGM_FI_DEV_GPU_UTIL\{.*namespace="`+namespace+`"`),
				"GPU metric should show namespace of allocated pod")
			Expect(metrics).To(MatchRegexp(`DCGM_FI_DEV_GPU_UTIL\{.*pod="`+podName+`"`),
				"GPU metric should show name of allocated pod")
		})
	})
})

// Helper functions

// trackResourceClaimsForPod tracks ResourceClaims for a pod and adds them to testResourceClaims
// This is used in Eventually blocks to wait for ResourceClaims to be created
func trackResourceClaimsForPod(namespace, podName string, testResourceClaims *[]resourceClaimInfo) error {
	claimNames, err := getResourceClaimNameFromPod(namespace, podName)
	if err != nil {
		return err
	}
	if len(claimNames) == 0 {
		return fmt.Errorf("no ResourceClaims found for pod %s/%s", namespace, podName)
	}
	for _, claimName := range claimNames {
		*testResourceClaims = append(*testResourceClaims, resourceClaimInfo{
			namespace: namespace,
			name:      claimName,
		})
	}
	return nil
}

// trackResourceClaimsForPods tracks ResourceClaims for multiple pods
func trackResourceClaimsForPods(namespace string, podNames []string, testResourceClaims *[]resourceClaimInfo) error {
	for _, podName := range podNames {
		if err := trackResourceClaimsForPod(namespace, podName, testResourceClaims); err != nil {
			return err
		}
	}
	return nil
}

// parseLogsToEnvVars parses bash export format logs and returns a map of environment variables
func parseLogsToEnvVars(logs string) map[string]string {
	lines := strings.Split(logs, "\n")
	envVars := make(map[string]string)
	for _, line := range lines {
		key, value := parseBashExportLine(line)
		if key != "" {
			envVars[key] = value
		}
	}
	return envVars
}

// extractDeviceID converts a GPU device name to device ID format
// e.g., "gpu-12345678-1234-1234-1234-123456789abc" -> "gpu_12345678_1234_1234_1234_123456789abc"
// Note: We now keep the "gpu-" prefix (converted to "gpu_") to match the new environment variable format
func extractDeviceID(gpuDevice string) string {
	return strings.ReplaceAll(gpuDevice, "-", "_")
}

// setupTest applies a manifest and tracks the namespace for cleanup
func setupTest(manifestPath, namespace string, testNamespaces *[]string) {
	*testNamespaces = append(*testNamespaces, namespace)
	applyManifest(manifestPath)
}

// waitAndTrackResourceClaims waits for ResourceClaims to be created and tracks them
func waitAndTrackResourceClaims(namespace string, podNames []string, testResourceClaims *[]resourceClaimInfo) {
	Eventually(func() error {
		return trackResourceClaimsForPods(namespace, podNames, testResourceClaims)
	}).WithTimeout(30 * time.Second).Should(Succeed())
}

func getKubeConfig() (*rest.Config, error) {
	// Try in-cluster config first
	config, err := ctrl.GetConfig()
	if err == nil {
		return config, nil
	}

	// Fall back to kubeconfig file
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}

	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

func applyManifest(manifestPath string) {
	// Get the test directory (where this file is located)
	_, filename, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(filename)
	manifestFile := filepath.Join(testDir, manifestPath)

	cmd := exec.Command("kubectl", "apply", "-f", manifestFile)
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "Failed to apply manifest %s: %s", manifestPath, string(output))
}

func deleteResourceClaim(namespace, claimName string) {
	cmd := exec.Command("kubectl", "delete", "resourceclaim", claimName, "-n", namespace, "--ignore-not-found=true", "--wait=false")
	_ = cmd.Run() // Ignore errors during cleanup

	// Wait a bit for the claim to be fully deleted and GPU to be freed
	time.Sleep(2 * time.Second)
}

func deleteNamespace(namespace string) {
	cmd := exec.Command("kubectl", "delete", "namespace", namespace, "--ignore-not-found=true", "--wait=false")
	_ = cmd.Run() // Ignore errors during cleanup
}

// waitForResourceClaimAllocated waits for a ResourceClaim to be allocated
func waitForResourceClaimAllocated(namespace, claimName string, timeout time.Duration) error {
	// Check if ResourceClaim has allocation.devices.results (which indicates it's allocated)
	cmd := exec.Command("kubectl", "get", "resourceclaim", claimName, "-n", namespace, "-o", "jsonpath={.status.allocation.devices.results[*].driver}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get ResourceClaim %s/%s: %w", namespace, claimName, err)
	}
	if strings.TrimSpace(string(output)) == "" {
		return fmt.Errorf("ResourceClaim %s/%s is not allocated yet (no devices in allocation)", namespace, claimName)
	}
	return nil
}

// getResourceClaimNameFromPod gets the ResourceClaim name from a pod's resourceClaims
// It checks pod.Status.ResourceClaimStatuses first (for template-based claims), then falls back to pod.Spec.ResourceClaims
func getResourceClaimNameFromPod(namespace, podName string) ([]string, error) {
	pod, err := kubeClient.CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	claimNames := make([]string, 0)
	seen := make(map[string]bool)

	// First, check pod.Status.ResourceClaimStatuses for generated claims from templates
	for _, status := range pod.Status.ResourceClaimStatuses {
		if status.ResourceClaimName != nil && *status.ResourceClaimName != "" {
			if !seen[*status.ResourceClaimName] {
				claimNames = append(claimNames, *status.ResourceClaimName)
				seen[*status.ResourceClaimName] = true
			}
		}
	}

	// Then, check pod.Spec.ResourceClaims for direct references
	for _, claim := range pod.Spec.ResourceClaims {
		if claim.ResourceClaimName != nil && *claim.ResourceClaimName != "" {
			if !seen[*claim.ResourceClaimName] {
				claimNames = append(claimNames, *claim.ResourceClaimName)
				seen[*claim.ResourceClaimName] = true
			}
		}
		// Note: For template-based claims, the actual ResourceClaim name is already
		// in pod.Status.ResourceClaimStatuses, so we don't need to query by label here
	}

	return claimNames, nil
}

func waitForPodReady(namespace, podName string, timeout time.Duration) {
	// First, wait for ResourceClaims to be allocated
	Eventually(func() error {
		claimNames, err := getResourceClaimNameFromPod(namespace, podName)
		if err != nil {
			return err
		}
		if len(claimNames) == 0 {
			// No ResourceClaims, proceed with pod check
			return nil
		}
		for _, claimName := range claimNames {
			if err := waitForResourceClaimAllocated(namespace, claimName, 30*time.Second); err != nil {
				return err
			}
		}
		return nil
	}).WithTimeout(timeout).Should(Succeed(), "ResourceClaims for pod %s/%s should be allocated", namespace, podName)

	// Then wait for pod to be ready
	Eventually(func() error {
		pod, err := kubeClient.CoreV1().Pods(namespace).Get(context.Background(), podName, metav1.GetOptions{})
		if err != nil {
			return err
		}

		// Check for scheduling issues
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodScheduled && condition.Status != corev1.ConditionTrue {
				// Get pod events for more details
				cmd := exec.Command("kubectl", "get", "events", "-n", namespace, "--field-selector", fmt.Sprintf("involvedObject.name=%s", podName), "-o", "jsonpath={.items[*].message}")
				events, _ := cmd.CombinedOutput()
				return fmt.Errorf("pod %s/%s is not scheduled: %s. Events: %s", namespace, podName, condition.Reason, string(events))
			}
		}

		if pod.Status.Phase != corev1.PodRunning {
			// Get pod events for more details
			cmd := exec.Command("kubectl", "get", "events", "-n", namespace, "--field-selector", fmt.Sprintf("involvedObject.name=%s", podName), "-o", "jsonpath={.items[*].message}")
			events, _ := cmd.CombinedOutput()
			return fmt.Errorf("pod %s/%s is not running: %s. Events: %s", namespace, podName, pod.Status.Phase, string(events))
		}

		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status != corev1.ConditionTrue {
				return fmt.Errorf("pod %s/%s is not ready: %s", namespace, podName, condition.Reason)
			}
		}
		return nil
	}).WithTimeout(timeout).Should(Succeed(), "Pod %s/%s should be ready", namespace, podName)
}

func getPodLogs(namespace, podName, containerName string) string {
	Eventually(func() (string, error) {
		req := kubeClient.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
			Container: containerName,
		})
		logs, err := req.Stream(context.Background())
		if err != nil {
			return "", err
		}
		defer func() {
			if err := logs.Close(); err != nil {
				GinkgoWriter.Printf("Error closing logs stream: %v\n", err)
			}
		}()

		buf := make([]byte, 1024*1024) // 1MB buffer
		n, err := logs.Read(buf)
		if err != nil && err.Error() != "EOF" {
			return "", err
		}
		return string(buf[:n]), nil
	}).WithTimeout(30 * time.Second).ShouldNot(BeEmpty())

	req := kubeClient.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		Container: containerName,
	})
	logs, err := req.Stream(context.Background())
	Expect(err).NotTo(HaveOccurred())
	defer func() {
		if err := logs.Close(); err != nil {
			GinkgoWriter.Printf("Error closing logs stream: %v\n", err)
		}
	}()

	buf := make([]byte, 1024*1024) // 1MB buffer
	n, err := logs.Read(buf)
	Expect(err).NotTo(HaveOccurred())
	return string(buf[:n])
}

// parseBashExportLine parses a bash export line and extracts KEY=value pairs
// Handles formats like: declare -x KEY="value", KEY="value", or KEY=value
func parseBashExportLine(line string) (key, value string) {
	// Remove "declare -x " prefix if present
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "declare -x ")
	line = strings.TrimSpace(line)

	// Split on first = sign
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", ""
	}

	key = strings.TrimSpace(parts[0])
	value = strings.TrimSpace(parts[1])

	// Remove surrounding quotes if present
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}

	return key, value
}

func extractGPUEnvVars(logs string) []string {
	// Extract GPU device IDs from logs
	// Pattern can be: declare -x GPU_DEVICE_<id>="gpu-<uuid>" or GPU_DEVICE_<id>=gpu-<uuid>
	lines := strings.Split(logs, "\n")
	gpus := make([]string, 0)
	seen := make(map[string]bool)

	for _, line := range lines {
		// Try to parse as bash export format first
		key, value := parseBashExportLine(line)
		if key != "" && strings.HasPrefix(key, "GPU_DEVICE_") && strings.HasPrefix(value, "gpu-") {
			if !seen[value] {
				gpus = append(gpus, value)
				seen[value] = true
			}
			continue
		}

		// Fallback to direct pattern matching (for non-bash export format)
		re := regexp.MustCompile(`GPU_DEVICE_[a-z0-9_]+=["']?gpu-([a-z0-9-]+)["']?`)
		matches := re.FindStringSubmatch(line)
		if len(matches) >= 2 {
			gpuID := "gpu-" + matches[1]
			if !seen[gpuID] {
				gpus = append(gpus, gpuID)
				seen[gpuID] = true
			}
		}
	}
	return gpus
}

// verifyEnvVars verifies that the logs contain all expected environment variables.
// expected is a map where keys are environment variable names (can contain %s for deviceID placeholder)
// and values are Gomega matchers to validate the values.
// If gpuDevice is provided, %s in keys will be replaced with the device ID.
func verifyEnvVars(logs string, gpuDevice string, expected map[string]types.GomegaMatcher) {
	envVars := parseLogsToEnvVars(logs)
	deviceID := ""
	if gpuDevice != "" {
		deviceID = extractDeviceID(gpuDevice)
	}

	for keyTemplate, matcher := range expected {
		// Replace %s placeholder with deviceID if present
		key := keyTemplate
		if deviceID != "" && strings.Contains(keyTemplate, "%s") {
			key = fmt.Sprintf(keyTemplate, deviceID)
		}
		Expect(envVars).To(HaveKey(key), "Should contain %s", key)
		if matcher != nil {
			Expect(envVars[key]).To(matcher, "Value for %s should match expectation", key)
		}
	}
}

func verifyNvidiaSmiBinary(namespace, podName, containerName string) {
	// Check if binary exists
	cmd := exec.Command("kubectl", "exec", "-n", namespace, podName, "-c", containerName, "--", "test", "-f", "/bin/nvidia-smi")
	err := cmd.Run()
	Expect(err).NotTo(HaveOccurred(), "nvidia-smi binary should exist at /bin/nvidia-smi")

	// Check if binary is executable
	cmd = exec.Command("kubectl", "exec", "-n", namespace, podName, "-c", containerName, "--", "test", "-x", "/bin/nvidia-smi")
	err = cmd.Run()
	Expect(err).NotTo(HaveOccurred(), "nvidia-smi binary should be executable")

	// Try to run nvidia-smi (it should not fail with "command not found")
	cmd = exec.Command("kubectl", "exec", "-n", namespace, podName, "-c", containerName, "--", "/bin/nvidia-smi", "--help")
	output, _ := cmd.CombinedOutput()
	// We expect either success or a usage error, but not "command not found"
	Expect(string(output)).NotTo(ContainSubstring("command not found"), "nvidia-smi should be available")
}

// runNvidiaSmi runs nvidia-smi in a pod and returns the output
func runNvidiaSmi(namespace, podName, containerName string) string {
	cmd := exec.Command("kubectl", "exec", "-n", namespace, podName, "-c", containerName, "--", "/bin/nvidia-smi")
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "nvidia-smi should run successfully: %s", string(output))
	return string(output)
}

// getNvidiaSmiUtilization runs nvidia-smi with debug flag and extracts the GPU utilization percentage
func getNvidiaSmiUtilization(namespace, podName, containerName string) int {
	cmd := exec.Command("kubectl", "exec", "-n", namespace, podName, "-c", containerName, "--", "/bin/nvidia-smi", "--debug")
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "nvidia-smi --debug should run successfully: %s", string(output))

	// Parse the debug output to find utilization
	// Format: "GPU stats - Index: 0, Used Memory: 40960.000000, Utilization: 75%"
	re := regexp.MustCompile(`Utilization: (\d+)%`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		// Fallback: try to parse from the main output (e.g., "75%")
		re = regexp.MustCompile(`(\d+)%\s+Default`)
		matches = re.FindStringSubmatch(string(output))
	}
	Expect(matches).To(HaveLen(2), "Should find utilization in nvidia-smi output: %s", string(output))

	util, err := strconv.Atoi(matches[1])
	Expect(err).NotTo(HaveOccurred(), "Should parse utilization as integer")
	return util
}

// applyManifestWithNamespace applies a manifest but replaces the namespace
func applyManifestWithNamespace(manifestPath, namespace string) {
	// First, create the namespace
	cmd := exec.Command("kubectl", "create", "namespace", namespace, "--dry-run=client", "-o", "yaml")
	nsYaml, err := cmd.Output()
	Expect(err).NotTo(HaveOccurred(), "Should create namespace YAML")

	cmd = exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(string(nsYaml))
	output, err := cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "Should create namespace: %s", string(output))

	// Read the manifest file
	manifestBytes, err := os.ReadFile(manifestPath)
	Expect(err).NotTo(HaveOccurred(), "Should read manifest file")

	// Replace the original namespace with the new one
	manifest := string(manifestBytes)
	// Replace namespace in metadata
	manifest = strings.ReplaceAll(manifest, "namespace: gpu-test-util", "namespace: "+namespace)
	// Also need to create ResourceClaimTemplate in the new namespace
	manifest = strings.ReplaceAll(manifest, "name: gpu-test-util", "name: "+namespace)

	// Apply the modified manifest
	cmd = exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	output, err = cmd.CombinedOutput()
	Expect(err).NotTo(HaveOccurred(), "Should apply manifest: %s", string(output))
}

// getPrometheusMetrics fetches Prometheus metrics from the nvidia-dcgm-exporter service
func getPrometheusMetrics() string {
	// Use kubectl to run a curl command in a temporary pod to access the service
	// This avoids needing port-forward which can be flaky in tests
	cmd := exec.Command("kubectl", "run", "metrics-test", "--rm", "-i", "--restart=Never",
		"--image=curlimages/curl:latest", "-n", "gpu-operator",
		"--", "curl", "-s", "http://nvidia-dcgm-exporter.gpu-operator:9400/metrics")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// If the curl pod approach fails, try getting the pod IP directly
		podIPCmd := exec.Command("kubectl", "get", "pod", "-l", "app=nvidia-dcgm-exporter",
			"-n", "gpu-operator", "-o", "jsonpath={.items[0].status.podIP}")
		podIP, podErr := podIPCmd.Output()
		if podErr == nil && len(podIP) > 0 {
			cmd = exec.Command("kubectl", "run", "metrics-test-2", "--rm", "-i", "--restart=Never",
				"--image=curlimages/curl:latest", "-n", "gpu-operator",
				"--", "curl", "-s", fmt.Sprintf("http://%s:9400/metrics", string(podIP)))
			output, err = cmd.CombinedOutput()
		}
	}
	Expect(err).NotTo(HaveOccurred(), "Should fetch Prometheus metrics: %s", string(output))
	return string(output)
}
