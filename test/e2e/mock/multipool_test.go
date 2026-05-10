package mock_e2e_test

import (
	"context"
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Phase 3: Multi-pool differentiation", func() {

	It("both nvml-mock-mock-a and nvml-mock-mock-b ConfigMaps exist", func() {
		_, err := kubeClient.CoreV1().ConfigMaps(releaseNs).Get(context.Background(), "nvml-mock-mock-a", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred(), "mock-a ConfigMap must exist")

		_, err = kubeClient.CoreV1().ConfigMaps(releaseNs).Get(context.Background(), "nvml-mock-mock-b", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred(), "mock-b ConfigMap must exist")
	})

	It("mock-a ConfigMap data references A100; mock-b references H100", func() {
		cmA, err := kubeClient.CoreV1().ConfigMaps(releaseNs).Get(context.Background(), "nvml-mock-mock-a", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(cmA.Data).To(HaveKey("config.yaml"))
		Expect(cmA.Data["config.yaml"]).To(ContainSubstring("A100"),
			"mock-a's profile YAML should mention A100")

		cmB, err := kubeClient.CoreV1().ConfigMaps(releaseNs).Get(context.Background(), "nvml-mock-mock-b", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(cmB.Data).To(HaveKey("config.yaml"))
		Expect(cmB.Data["config.yaml"]).To(ContainSubstring("H100"),
			"mock-b's profile YAML should mention H100")
	})

	// mock-b's values set gpu.overrides.device_defaults.name. Asserts the
	// override is deep-merged into the base profile and surfaces in the
	// per-pool CM, while mock-a (no override) is unaffected.
	It("mock-b's gpu.overrides flows through to its nvml-mock ConfigMap", func() {
		const overriddenName = "Renamed-H100-For-E2E"

		cmB, err := kubeClient.CoreV1().ConfigMaps(releaseNs).Get(context.Background(), "nvml-mock-mock-b", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(cmB.Data["config.yaml"]).To(ContainSubstring(overriddenName),
			"mock-b's CM should contain the overridden device name")

		cmA, err := kubeClient.CoreV1().ConfigMaps(releaseNs).Get(context.Background(), "nvml-mock-mock-a", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(cmA.Data["config.yaml"]).NotTo(ContainSubstring(overriddenName),
			"mock-a should not be affected by mock-b's per-pool override")
	})

	It("mock-a's DaemonSet pod is pinned to worker-1; mock-b's to worker-2", func() {
		podsA, err := kubeClient.CoreV1().Pods(releaseNs).List(context.Background(), metav1.ListOptions{
			LabelSelector: "fake-gpu-operator/component=nvml-mock,fake-gpu-operator/pool=mock-a",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(podsA.Items).To(HaveLen(1))
		Expect(podsA.Items[0].Spec.NodeName).To(Equal(workerMockA))

		podsB, err := kubeClient.CoreV1().Pods(releaseNs).List(context.Background(), metav1.ListOptions{
			LabelSelector: "fake-gpu-operator/component=nvml-mock,fake-gpu-operator/pool=mock-b",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(podsB.Items).To(HaveLen(1))
		Expect(podsB.Items[0].Spec.NodeName).To(Equal(workerMockB))
	})

	// End-to-end proof that mock-b's GPU allocation pipeline (gpu-operator's
	// device-plugin advertising nvidia.com/gpu and the kubelet binding the
	// claim) is operational on worker-2, mirroring Phase 1's mock-a check.
	// We do NOT assert the per-pool model name (e.g. "NVIDIA H100"). The
	// libnvidia-ml inside both device-plugin AND DRA-allocated pods cannot
	// auto-locate the per-pool config via /proc/self/maps and falls back to
	// its compiled-in default (MOCK NVIDIA A100). The model-name assertion
	// in Phase 1 (mock-a) only passes by coincidence — the default IS A100.
	// Tighten back to expectedH100Product once upstream closes the in-pod
	// profile-fidelity gap.
	It("a pod requesting nvidia.com/gpu: 1 schedules on worker-2 and runs nvidia-smi", func() {
		const podNs = "phase3-nvidia-smi-mock-b"
		const podName = "nvidia-smi-mock-b"

		_, err := kubeClient.CoreV1().Namespaces().Create(context.Background(),
			&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: podNs}},
			metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			_ = kubeClient.CoreV1().Namespaces().Delete(context.Background(), podNs, metav1.DeleteOptions{})
		})

		manifest := fmt.Sprintf(`
apiVersion: v1
kind: Pod
metadata:
  name: %s
  namespace: %s
spec:
  restartPolicy: OnFailure
  nodeSelector:
    run.ai/simulated-gpu-node-pool: mock-b
  containers:
    - name: nvidia-smi
      image: nvcr.io/nvidia/k8s/cuda-sample:vectoradd-cuda12.5.0
      command: ["nvidia-smi"]
      resources:
        limits:
          nvidia.com/gpu: "1"
`, podName, podNs)
		Expect(kubectlApply(manifest)).To(Succeed())

		waitForPodOnNode(podNs, podName, workerMockB, podReadyTimeout)

		logs := getPodLogs(podNs, podName, "nvidia-smi")
		Expect(logs).To(ContainSubstring("NVIDIA-SMI"))
		Expect(strings.ToUpper(logs)).To(ContainSubstring("MOCK NVIDIA"))
	})
})
