package mock_e2e_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Phase 4: Fake + mock coexistence", func() {

	It("KWOK fake node kwok-fake-1 exists with the fake-default pool label", func() {
		node, err := kubeClient.CoreV1().Nodes().Get(context.Background(), fakeNodeName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(node.Labels["run.ai/simulated-gpu-node-pool"]).To(Equal(poolFakeDefault))
		Expect(node.Annotations["kwok.x-k8s.io/node"]).To(Equal("fake"))
	})

	It("MockController did NOT create a nvml-mock-fake-default DaemonSet", func() {
		_, err := kubeClient.AppsV1().DaemonSets(releaseNs).Get(context.Background(), "nvml-mock-fake-default", metav1.GetOptions{})
		Expect(resourceMissing(err)).To(BeTrue(),
			"nvml-mock-fake-default DaemonSet must NOT exist; mock controller should skip fake pools")
	})

	It("MockController did NOT create a nvml-mock-fake-default ConfigMap", func() {
		_, err := kubeClient.CoreV1().ConfigMaps(releaseNs).Get(context.Background(), "nvml-mock-fake-default", metav1.GetOptions{})
		Expect(resourceMissing(err)).To(BeTrue(),
			"nvml-mock-fake-default ConfigMap must NOT exist")
	})

	It("status-updater created a node-topology ConfigMap for kwok-fake-1 (existing fake-pool behavior preserved)", func() {
		Eventually(func() error {
			cms, err := kubeClient.CoreV1().ConfigMaps(releaseNs).List(context.Background(), metav1.ListOptions{
				LabelSelector: fmt.Sprintf("node-name=%s", fakeNodeName),
			})
			if err != nil {
				return err
			}
			if len(cms.Items) == 0 {
				return fmt.Errorf("no topology CM yet for %s", fakeNodeName)
			}
			return nil
		}, 60*time.Second, 2*time.Second).Should(Succeed())
	})

	It("a pod with nvidia.com/gpu and fake-default nodeSelector schedules on the KWOK fake node", func() {
		const podNs = "phase4-coexistence"
		const podName = "fake-pool-pod"

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
    run.ai/simulated-gpu-node-pool: %s
  tolerations:
    - key: kwok.x-k8s.io/node
      operator: Equal
      value: "fake"
      effect: NoSchedule
  containers:
    - name: app
      image: ubuntu:22.04
      command: ["sleep", "infinity"]
      resources:
        limits:
          nvidia.com/gpu: "1"
`, podName, podNs, poolFakeDefault)
		Expect(kubectlApply(manifest)).To(Succeed())

		// KWOK simulates the binding instantly. Just verify NodeName.
		Eventually(func() string {
			p, err := kubeClient.CoreV1().Pods(podNs).Get(context.Background(), podName, metav1.GetOptions{})
			if err != nil {
				return ""
			}
			return p.Spec.NodeName
		}, 30*time.Second, 1*time.Second).Should(Equal(fakeNodeName))
	})
})
