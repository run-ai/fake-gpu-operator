package upgrade_e2e_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	releaseNamespace = "gpu-operator"

	// Synonym of the singular ConfigMap created by templates/topology-cm.yml.
	// Its preservation across helm upgrade is the canary for this suite.
	topologyCMName = "topology"

	podReadyTimeout = 3 * time.Minute
)

var (
	kubeClient kubernetes.Interface
	restConfig *rest.Config
)

func TestUpgrade(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Helm Upgrade E2E Suite")
}

var _ = BeforeSuite(func() {
	var err error

	restConfig, err = getKubeConfig()
	Expect(err).NotTo(HaveOccurred())

	kubeClient, err = kubernetes.NewForConfig(restConfig)
	Expect(err).NotTo(HaveOccurred())

	// Sanity: cluster reachable and the release namespace exists (the
	// baseline install creates it).
	_, err = kubeClient.CoreV1().Namespaces().Get(context.Background(), releaseNamespace, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred(),
		"namespace %s must exist (setup.sh helm install)", releaseNamespace)
})

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
