package upgrade_e2e_test

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("Helm upgrade from baseline OCI release to local chart", func() {
	var preUpgradeTopologyUID types.UID

	BeforeEach(func() {
		// Capture pre-upgrade UID of the topology CM. After upgrade, an
		// unchanged UID proves the CM was preserved (not deleted+recreated)
		// — preserving it across upgrade is what real users expect.
		cm, err := kubeClient.CoreV1().ConfigMaps(releaseNamespace).Get(
			context.Background(), topologyCMName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred(),
			"baseline release must have created the %q ConfigMap", topologyCMName)
		preUpgradeTopologyUID = cm.UID
	})

	It("upgrades cleanly to the local chart with the same values", func() {
		localChartPath := filepath.Join(projectRoot, "deploy", "fake-gpu-operator")
		valuesPath := filepath.Join(projectRoot, "test", "e2e", "upgrade", "fixtures", "values-upgrade.yaml")
		dockerTag := "0.0.0-dev"

		// `helm dependency update` populates charts/ for the conditional
		// subcharts (gpuOperator / nvidiaDraDriver). Even though they render
		// to nothing here, helm refuses to upgrade with missing dep archives.
		runOrFail("helm", "dependency", "update", localChartPath)

		// The upgrade. If any template references a top-level value that
		// the baseline's stored values lack, this fails — which is the
		// regression class RUN-39195 exists to catch.
		runOrFail("helm", "upgrade", releaseName, localChartPath,
			"--namespace", releaseNamespace,
			"-f", valuesPath,
			"--set", "devicePlugin.image.tag="+dockerTag,
			"--set", "statusUpdater.image.tag="+dockerTag,
			"--set", "topologyServer.image.tag="+dockerTag,
			"--wait", "--timeout", helmUpgradeTimeout.String(),
		)

		By("verifying enabled component pods reach steady state")
		waitForPodReady("app=status-updater", podReadyTimeout)
		waitForPodReady("app=topology-server", podReadyTimeout)
		// status-updater labels worker nodes with
		// `nvidia.com/gpu.deploy.device-plugin=true` after start-up
		// (see internal/status-updater/handlers/node/labels.go). The
		// device-plugin DaemonSet selects on that label, so its pod count
		// goes from 0 to 1 a few seconds after status-updater is Ready.
		waitForDaemonSetReady("device-plugin", podReadyTimeout)

		By("verifying the topology ConfigMap was preserved across upgrade")
		postCM, err := kubeClient.CoreV1().ConfigMaps(releaseNamespace).Get(
			context.Background(), topologyCMName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(postCM.UID).To(Equal(preUpgradeTopologyUID),
			"topology ConfigMap should be patched in place, not recreated")
		Expect(postCM.Data).To(HaveKey("topology.yml"))

		By("verifying helm sees the release as deployed (not failed/pending)")
		out := runOrFail("helm", "list", "--namespace", releaseNamespace, "-o", "json")
		Expect(out).To(ContainSubstring(`"status":"deployed"`),
			"helm release should be in 'deployed' state after upgrade")
	})
})

// runOrFail shells out to a command, streams output to GinkgoWriter, and
// fails the spec with the captured output if the command exits non-zero.
func runOrFail(name string, args ...string) string {
	GinkgoWriter.Printf("$ %s %s\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	GinkgoWriter.Print(string(out))
	Expect(err).NotTo(HaveOccurred(),
		"command failed: %s %s\n--- output ---\n%s", name, strings.Join(args, " "), string(out))
	return string(out)
}

// waitForPodReady polls until every pod matching the label selector in
// releaseNamespace is Running + Ready (mirrors mock suite helper).
func waitForPodReady(labelSelector string, timeout time.Duration) {
	Eventually(func() error {
		pods, err := kubeClient.CoreV1().Pods(releaseNamespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			return err
		}
		if len(pods.Items) == 0 {
			return fmt.Errorf("no pods match selector %q", labelSelector)
		}
		for _, p := range pods.Items {
			if p.Status.Phase != corev1.PodRunning {
				return fmt.Errorf("pod %s phase %s (want Running)", p.Name, p.Status.Phase)
			}
			ready := false
			for _, c := range p.Status.Conditions {
				if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
					ready = true
					break
				}
			}
			if !ready {
				return fmt.Errorf("pod %s not Ready", p.Name)
			}
		}
		return nil
	}, timeout, 2*time.Second).Should(Succeed(), "no Ready pods for selector %q", labelSelector)
}

// waitForDaemonSetReady polls until the named DaemonSet reports
// numberReady == desiredNumberScheduled (and desired > 0).
func waitForDaemonSetReady(name string, timeout time.Duration) {
	Eventually(func() error {
		ds, err := kubeClient.AppsV1().DaemonSets(releaseNamespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if ds.Status.DesiredNumberScheduled == 0 {
			return fmt.Errorf("DaemonSet %s desiredNumberScheduled=0 (no matching nodes?)", name)
		}
		if ds.Status.NumberReady != ds.Status.DesiredNumberScheduled {
			return fmt.Errorf("DaemonSet %s: %d/%d ready", name, ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)
		}
		return nil
	}, timeout, 2*time.Second).Should(Succeed())
}
