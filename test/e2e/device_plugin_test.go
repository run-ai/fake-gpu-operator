package e2e_test

import (
	"context"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Regression guard for RUN-39004: covers the legacy (non-DRA) device-plugin path, where
// a pod gets its GPU via the nvidia.com/gpu extended resource and runs the fake nvidia-smi.
var _ = Describe("Device-Plugin Path Tests", func() {
	var testNamespaces []string

	BeforeEach(func() {
		// Skip in suites that don't enable the device-plugin (e.g. make e2e-profiles).
		_, err := kubeClient.AppsV1().DaemonSets("gpu-operator").Get(context.Background(), "device-plugin", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			Skip("device-plugin daemonset not deployed (devicePlugin.enabled=false) — skipping legacy device-plugin path test")
		}
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		for _, ns := range testNamespaces {
			deleteNamespace(ns)
		}
		testNamespaces = nil
	})

	Describe("nvidia-smi on a non-DRA GPU pod", func() {
		It("runs /bin/nvidia-smi without panicking and reports the pool's GPU", func() {
			manifestPath := filepath.Join("fixtures", "manifests", "device-plugin-pod.yaml")
			namespace := "gpu-test-device-plugin"
			podName := "device-plugin-pod0"

			setupTest(manifestPath, namespace, &testNamespaces)
			waitForPodReady(namespace, podName, podReadyTimeout)

			verifyNvidiaSmiBinary(namespace, podName, "ctr0")

			output := runNvidiaSmi(namespace, podName, "ctr0")
			Expect(output).NotTo(ContainSubstring("panic"))
			Expect(strings.ToUpper(output)).To(ContainSubstring("NVIDIA-SMI"))
			// Product column is truncated to 12 chars by sizeString, so match the prefix.
			Expect(output).To(ContainSubstring(expectedGpuProduct[:10]))
			// 40960 = default pool gpuMemory; proves nvidia-smi resolved this node's topology.
			Expect(output).To(ContainSubstring("40960MiB"))
		})
	})
})
