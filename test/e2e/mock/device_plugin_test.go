package mock_e2e_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Phase 1: Device-plugin path (worker-1 / mock-a / a100)", func() {

	It("nvml-mock-mock-a DaemonSet has 1 Ready pod on worker-1", func() {
		Eventually(func() error {
			ds, err := kubeClient.AppsV1().DaemonSets(releaseNs).Get(context.Background(), "nvml-mock-mock-a", metav1.GetOptions{})
			if err != nil {
				return err
			}
			if ds.Status.NumberReady != 1 {
				return fmt.Errorf("nvml-mock-mock-a NumberReady=%d (want 1)", ds.Status.NumberReady)
			}
			return nil
		}, 90*time.Second, 2*time.Second).Should(Succeed())
	})

	It("mock NVML library exists on worker-1's host filesystem", func() {
		out, err := kindNodeExec(workerMockA, "ls", "/var/lib/nvml-mock/driver/usr/lib64/")
		Expect(err).NotTo(HaveOccurred(), "kind exec failed: %s", out)
		Expect(out).To(ContainSubstring("libnvidia-ml.so"))
	})

	It("worker-1 has nvidia.com/gpu.present=true label", func() {
		Eventually(func() string {
			node, err := kubeClient.CoreV1().Nodes().Get(context.Background(), workerMockA, metav1.GetOptions{})
			if err != nil {
				return ""
			}
			return node.Labels["nvidia.com/gpu.present"]
		}, 60*time.Second, 2*time.Second).Should(Equal("true"))
	})

	// In gpu-operator v26.3.1 the legacy 'gpu-operator-validator' Job was replaced
	// by the 'nvidia-operator-validator' DaemonSet; assert the DaemonSet has at
	// least one Ready pod (which means all four init phases — driver/toolkit/cuda/plugin —
	// completed successfully).
	It("nvidia-operator-validator DaemonSet has Ready pods", func() {
		Eventually(func() error {
			ds, err := kubeClient.AppsV1().DaemonSets(releaseNs).Get(context.Background(), "nvidia-operator-validator", metav1.GetOptions{})
			if err != nil {
				return err
			}
			if ds.Status.NumberReady < 1 {
				return fmt.Errorf("nvidia-operator-validator NumberReady=%d (want ≥1)", ds.Status.NumberReady)
			}
			return nil
		}, 5*time.Minute, 5*time.Second).Should(Succeed())
	})

	It("worker-1 reports nvidia.com/gpu: 8 allocatable (a100 profile)", func() {
		Eventually(func() string {
			node, err := kubeClient.CoreV1().Nodes().Get(context.Background(), workerMockA, metav1.GetOptions{})
			if err != nil {
				return ""
			}
			q, ok := node.Status.Allocatable["nvidia.com/gpu"]
			if !ok {
				return ""
			}
			return q.String()
		}, 3*time.Minute, 5*time.Second).Should(Equal("8"))
	})

	It("a pod requesting nvidia.com/gpu: 1 schedules on worker-1 and runs nvidia-smi successfully", func() {
		const podNs = "phase1-nvidia-smi"
		const podName = "nvidia-smi-mock-a"

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
    run.ai/simulated-gpu-node-pool: mock-a
  containers:
    - name: nvidia-smi
      image: nvcr.io/nvidia/k8s/cuda-sample:vectoradd-cuda12.5.0
      command: ["nvidia-smi"]
      resources:
        limits:
          nvidia.com/gpu: "1"
`, podName, podNs)
		Expect(kubectlApply(manifest)).To(Succeed())

		waitForPodOnNode(podNs, podName, workerMockA, podReadyTimeout)

		logs := getPodLogs(podNs, podName, "nvidia-smi")
		Expect(strings.ToUpper(logs)).To(ContainSubstring(strings.ToUpper(expectedA100Product)))
	})
})
