package kai_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func TestFractionalGpuScheduling(t *testing.T) {
	if os.Getenv("KAI_E2E") != "true" {
		t.Skip("run with make test-e2e-kai against the KIND cluster")
	}

	clusterName := os.Getenv("KIND_CLUSTER_NAME")
	if clusterName == "" {
		clusterName = "fake-gpu-operator-kai"
	}
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})
	rawConfig, err := kubeconfig.RawConfig()
	if err != nil {
		t.Fatal(err)
	}
	if rawConfig.CurrentContext != "kind-"+clusterName {
		t.Fatalf("current Kubernetes context is %q, want %q", rawConfig.CurrentContext, "kind-"+clusterName)
	}
	config, err := kubeconfig.ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	worker := waitForGpuWorker(t, ctx, client)
	slices, err := client.ResourceV1().ResourceSlices().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, slice := range slices.Items {
		if slice.Spec.Driver == "gpu.nvidia.com" && slice.Spec.NodeName != nil && *slice.Spec.NodeName == worker.Name {
			t.Fatalf("worker %s has GPU DRA ResourceSlice %s; fractional KAI scheduling requires the legacy device plugin", worker.Name, slice.Name)
		}
	}
	hostname := worker.Labels["kubernetes.io/hostname"]
	if hostname == "" {
		t.Fatalf("GPU worker %s has no kubernetes.io/hostname label", worker.Name)
	}

	namespace, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "kai-fraction-e2e-"},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.CoreV1().Namespaces().Delete(ctx, namespace.Name, metav1.DeleteOptions{}); err != nil {
			t.Logf("delete namespace %s: %v", namespace.Name, err)
		}
	})

	for i := 0; i < 4; i++ {
		createFractionPod(t, ctx, client, namespace.Name, fmt.Sprintf("half-gpu-%d", i), hostname)
	}
	for i := 0; i < 4; i++ {
		waitForPodRunning(t, ctx, client, namespace.Name, fmt.Sprintf("half-gpu-%d", i), worker.Name, 5*time.Minute)
	}
	waitForReservations(t, ctx, client, worker.Name, 2, 2*time.Minute)

	createFractionPod(t, ctx, client, namespace.Name, "overflow", hostname)
	for end := time.Now().Add(30 * time.Second); time.Now().Before(end); {
		pod, err := client.CoreV1().Pods(namespace.Name).Get(ctx, "overflow", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if pod.Spec.NodeName != "" {
			t.Fatalf("overflow pod scheduled to %s although four half-GPU pods occupy both GPUs", pod.Spec.NodeName)
		}
		time.Sleep(2 * time.Second)
	}

	if err := client.CoreV1().Pods(namespace.Name).Delete(ctx, "half-gpu-0", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	waitForPodRunning(t, ctx, client, namespace.Name, "overflow", worker.Name, 5*time.Minute)
}

func waitForGpuWorker(t *testing.T, ctx context.Context, client kubernetes.Interface) *corev1.Node {
	t.Helper()
	var last string
	for end := time.Now().Add(3 * time.Minute); time.Now().Before(end); {
		nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{LabelSelector: "run.ai/simulated-gpu-node-pool=default"})
		if err != nil {
			last = err.Error()
		} else {
			for i := range nodes.Items {
				node := &nodes.Items[i]
				if node.Annotations["kwok.x-k8s.io/node"] != "" {
					continue
				}
				gpuCount := node.Status.Allocatable[corev1.ResourceName("nvidia.com/gpu")]
				if gpuCount.Value() != 2 {
					last = fmt.Sprintf("node %s has %s allocatable GPUs, want 2", node.Name, gpuCount.String())
					continue
				}
				return node
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("GPU worker was not ready: %s", last)
	return nil
}

func createFractionPod(t *testing.T, ctx context.Context, client kubernetes.Interface, namespace, name, hostname string) {
	t.Helper()
	_, err := client.CoreV1().Pods(namespace).Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"kai.scheduler/queue": "default-queue",
			},
			Annotations: map[string]string{
				"gpu-fraction": "0.5",
			},
		},
		Spec: corev1.PodSpec{
			SchedulerName: "kai-scheduler",
			NodeSelector:  map[string]string{"kubernetes.io/hostname": hostname},
			Containers: []corev1.Container{{
				Name:  "workload",
				Image: "registry.k8s.io/pause:3.10.1",
			}},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
}

func waitForPodRunning(t *testing.T, ctx context.Context, client kubernetes.Interface, namespace, name, nodeName string, timeout time.Duration) {
	t.Helper()
	var last string
	for end := time.Now().Add(timeout); time.Now().Before(end); {
		pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			last = err.Error()
		} else {
			last = fmt.Sprintf("phase=%s node=%s conditions=%v", pod.Status.Phase, pod.Spec.NodeName, pod.Status.Conditions)
			if pod.Spec.NodeName == nodeName && pod.Status.Phase == corev1.PodRunning {
				return
			}
		}
		time.Sleep(2 * time.Second)
	}
	events, _ := client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{FieldSelector: "involvedObject.name=" + name})
	var messages []string
	if events != nil {
		for _, event := range events.Items {
			messages = append(messages, event.Reason+": "+event.Message)
		}
	}
	t.Fatalf("pod %s did not run on %s: %s; events: %s", name, nodeName, last, strings.Join(messages, "; "))
}

func waitForReservations(t *testing.T, ctx context.Context, client kubernetes.Interface, nodeName string, want int, timeout time.Duration) {
	t.Helper()
	var last string
	for end := time.Now().Add(timeout); time.Now().Before(end); {
		pods, err := client.CoreV1().Pods("kai-resource-reservation").List(ctx, metav1.ListOptions{})
		if err != nil {
			last = err.Error()
		} else {
			count := 0
			for _, pod := range pods.Items {
				if pod.Spec.NodeName != nodeName || pod.Status.Phase != corev1.PodRunning {
					continue
				}
				for _, container := range pod.Spec.Containers {
					gpuLimit := container.Resources.Limits[corev1.ResourceName("nvidia.com/gpu")]
					if gpuLimit.Value() == 1 {
						count++
						break
					}
				}
			}
			last = fmt.Sprintf("%d running GPU reservation pods on %s, want %d", count, nodeName, want)
			if count == want {
				return
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatal(last)
}
