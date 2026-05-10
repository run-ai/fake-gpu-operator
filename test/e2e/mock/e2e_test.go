package mock_e2e_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	testTimeout      = 5 * time.Minute
	podReadyTimeout  = 3 * time.Minute
	namespaceTimeout = 30 * time.Second

	// Pool names match values-mock.yaml's topology.nodePools keys.
	poolMockA       = "mock-a"
	poolMockB       = "mock-b"
	poolFakeDefault = "fake-default"

	// Node names match kind-cluster-config.yaml + setup.sh's KWOK injection.
	workerMockA  = "mock-cluster-worker"
	workerMockB  = "mock-cluster-worker2"
	fakeNodeName = "kwok-fake-1"

	// Namespace where helm installs everything.
	releaseNs = "gpu-operator"

	// Profile-derived expected values (mock-a uses a100, mock-b uses h100).
	expectedA100Product = "NVIDIA A100"
	expectedH100Product = "NVIDIA H100"
)

var (
	kubeClient kubernetes.Interface
	restConfig *rest.Config
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mock E2E Suite")
}

var _ = BeforeSuite(func() {
	var err error

	restConfig, err = getKubeConfig()
	Expect(err).NotTo(HaveOccurred())

	kubeClient, err = kubernetes.NewForConfig(restConfig)
	Expect(err).NotTo(HaveOccurred())

	// Sanity: cluster reachable, both real workers present, KWOK fake node present.
	nodes, err := kubeClient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	nodeNames := map[string]bool{}
	for _, n := range nodes.Items {
		nodeNames[n.Name] = true
	}
	Expect(nodeNames).To(HaveKey(workerMockA), "real worker for mock-a must exist (set up by setup.sh)")
	Expect(nodeNames).To(HaveKey(workerMockB), "real worker for mock-b must exist (set up by setup.sh)")
	Expect(nodeNames).To(HaveKey(fakeNodeName), "KWOK fake node must exist (injected by setup.sh)")
})

// getKubeConfig loads kubeconfig from KUBECONFIG env or ~/.kube/config; the
// kubeconfig points at the kind-mock-cluster context which setup.sh sets up.
func getKubeConfig() (*rest.Config, error) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		kubeconfig = filepath.Join(home, ".kube", "config")
	}
	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig from %q: %w", kubeconfig, err)
	}
	return cfg, nil
}

// silence unused-import warnings on packages used by other test files in the
// package. Without this, `go build ./...` complains until the phase test files
// are added.
var _ = corev1.Pod{}
