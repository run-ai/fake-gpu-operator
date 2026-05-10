package mock_e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// kindNodeExec runs `docker exec <node> <cmd...>` against the kind node
// container. Used to verify on-host artifacts (mock NVML library, /dev/nvidiaX).
func kindNodeExec(nodeContainerName string, cmd ...string) (string, error) {
	args := append([]string{"exec", nodeContainerName}, cmd...)
	out, err := exec.Command("docker", args...).CombinedOutput()
	return string(out), err
}

// waitForPodReady waits until at least one pod matching the label selector is
// Ready in the namespace, or the timeout fires.
func waitForPodReady(ns, labelSelector string, timeout time.Duration) {
	Eventually(func() error {
		pods, err := kubeClient.CoreV1().Pods(ns).List(context.Background(), metav1.ListOptions{
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

// waitForPodOnNode waits until the named pod has reached Running OR Succeeded
// on the expected node. Succeeded is accepted because short-lived commands
// (e.g. `nvidia-smi` then exit) may transition through Running too quickly to
// catch on a polling cycle, but they leave logs behind for assertions.
func waitForPodOnNode(ns, podName, expectedNode string, timeout time.Duration) {
	Eventually(func() error {
		p, err := kubeClient.CoreV1().Pods(ns).Get(context.Background(), podName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if p.Spec.NodeName != expectedNode {
			return fmt.Errorf("pod %s scheduled to %s (want %s)", podName, p.Spec.NodeName, expectedNode)
		}
		switch p.Status.Phase {
		case corev1.PodRunning, corev1.PodSucceeded:
			return nil
		case corev1.PodFailed:
			return fmt.Errorf("pod %s failed", podName)
		default:
			return fmt.Errorf("pod %s phase %s (want Running or Succeeded)", podName, p.Status.Phase)
		}
	}, timeout, 2*time.Second).Should(Succeed())
}

// getPodLogs returns the logs of the named container in the pod.
func getPodLogs(ns, podName, container string) string {
	req := kubeClient.CoreV1().Pods(ns).GetLogs(podName, &corev1.PodLogOptions{Container: container})
	stream, err := req.Stream(context.Background())
	Expect(err).NotTo(HaveOccurred())
	defer func() {
		if err := stream.Close(); err != nil {
			GinkgoWriter.Printf("closing pod-log stream: %v\n", err)
		}
	}()

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(stream)
	Expect(err).NotTo(HaveOccurred())
	return buf.String()
}

// kubectlApply shells out to `kubectl apply -f -` with stdin = manifest.
func kubectlApply(manifest string) error {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kubectl apply: %w (output: %s)", err, string(out))
	}
	return nil
}

// resourceMissing returns true if the kube error indicates NotFound.
func resourceMissing(err error) bool {
	return err != nil && apierrors.IsNotFound(err)
}
