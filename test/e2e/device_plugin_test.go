package e2e_test

import (
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Covers the legacy (non-DRA) device-plugin path, which the rest of this suite does not
// exercise: it allocates GPUs via the nvidia.com/gpu extended resource served by the fake
// RealNodeDevicePlugin on the real worker, rather than via ResourceClaims on KWOK nodes.
//
// Regression guard for RUN-39004: before the fix, Allocate() didn't inject NODE_NAME, so
// the in-container /bin/nvidia-smi queried an empty node, got a non-JSON error from
// topology-server, and panicked ("invalid character 'C' looking for beginning of value").
var _ = Describe("Device-Plugin Path Tests", func() {
	var testNamespaces []string

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
			// No ResourceClaims on this path — waitForPodReady handles that and just waits
			// for the pod to be Running/Ready once the device-plugin allocates the GPU.
			waitForPodReady(namespace, podName, podReadyTimeout)

			verifyNvidiaSmiBinary(namespace, podName, "ctr0")

			// runNvidiaSmi asserts a zero exit code, so a panic (the RUN-39004 bug) fails here.
			output := runNvidiaSmi(namespace, podName, "ctr0")
			Expect(output).NotTo(ContainSubstring("panic"), "nvidia-smi must not panic (RUN-39004)")
			Expect(output).NotTo(ContainSubstring("invalid character"), "nvidia-smi must not hit the JSON-decode panic (RUN-39004)")
			Expect(strings.ToUpper(output)).To(ContainSubstring("NVIDIA-SMI"), "nvidia-smi should render its header")
			// The product column is truncated to 12 chars by sizeString, so match the prefix.
			Expect(output).To(ContainSubstring(expectedGpuProduct[:10]), "nvidia-smi should report the default pool's GPU product")
			// 40960 is the default pool's gpuMemory; rendering it proves nvidia-smi resolved
			// THIS node's topology, which only works because NODE_NAME was injected (RUN-39004).
			Expect(output).To(ContainSubstring("40960MiB"), "nvidia-smi should report the default pool's GPU memory")
		})
	})
})
