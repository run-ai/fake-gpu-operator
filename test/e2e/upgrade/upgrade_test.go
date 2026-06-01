package upgrade_e2e_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// markerFile is where scripts/apply-upgrade.sh writes the pre-upgrade
// topology ConfigMap UID. The Ginkgo suite reads it to verify the CM was
// patched in place (UID unchanged), not deleted+recreated, by the upgrade.
const markerFile = "/tmp/upgrade-cluster-pre-upgrade-cm-uid"

var _ = Describe("Helm upgrade from baseline OCI release to local chart", func() {
	It("leaves the release healthy and the topology ConfigMap preserved", func() {
		By("reading pre-upgrade CM UID written by apply-upgrade.sh")
		raw, err := os.ReadFile(markerFile)
		Expect(err).NotTo(HaveOccurred(),
			"marker file %s missing — has apply-upgrade.sh run?", markerFile)
		preUpgradeUID := types.UID(strings.TrimSpace(string(raw)))
		Expect(preUpgradeUID).NotTo(BeEmpty(), "marker file should contain a UID")

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
		Expect(postCM.UID).To(Equal(preUpgradeUID),
			"topology ConfigMap should be patched in place, not recreated")
		Expect(postCM.Data).To(HaveKey("topology.yml"))

		By("verifying helm sees the release as deployed (not failed/pending)")
		out := runOrFail("helm", "-n", releaseNamespace, "list", "-o", "json")
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
