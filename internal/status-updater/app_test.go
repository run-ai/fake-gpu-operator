package status_updater_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	status_updater "github.com/run-ai/gpu-mock-stack/internal/status-updater"
)

func TestStatusUpdater(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "StatusUpdater Suite")
}

var _ = Describe("StatusUpdater", func() {
	const (
		fakeCmName      = "fake-cm-name"
		fakeCmNamespace = "fake-cm-namespace"
	)

	var (
		kubeclient kubernetes.Interface
	)

	BeforeEach(func() {
		topologyConfigMap := &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fakeCmName,
				Namespace: fakeCmNamespace,
			},
			Data: map[string]string{
				"topology.yml": `
nodes:
  worker-1:
    gpu-memory: 11441
    gpu-product: Tesla-K80
    gpu-count: 2
`,
			},
		}

		kubeclient = fake.NewSimpleClientset()

		kubeclient.CoreV1().ConfigMaps(fakeCmNamespace).Create(context.TODO(), topologyConfigMap, metav1.CreateOptions{})
		setupFakes(kubeclient)
		setupConfig(kubeclient)
	})

	It("should work", func() {
		go status_updater.Run()

		// Eventually the configmap should be updated
		Eventually(func() error {
			cm, err := kubeclient.CoreV1().ConfigMaps(fakeCmNamespace).Get(context.TODO(), fakeCmName, metav1.GetOptions{})
			if err != nil {
				return err
			}
			expected := `mig-strategy: ""
nodes:
    worker-1:
        gpu-count: 2
        gpu-memory: 11441
        gpu-product: Tesla-K80
        gpus:
            - id: gpu-0
              metrics:
                metadata:
                    namespace: ""
                    pod: ""
                    container: ""
                gpustatus:
                    utilization: 0
                    fb-used: 0
            - id: gpu-1
              metrics:
                metadata:
                    namespace: ""
                    pod: ""
                    container: ""
                gpustatus:
                    utilization: 0
                    fb-used: 0
`
			if cm.Data["topology.yml"] != expected {
				log.Printf("%s", cm.Data["topology.yml"])
				return fmt.Errorf("configmap data is not correct. got: \n%s\nexpected: \n%s\n", cm.Data["topology.yml"], expected)
			}
			return nil
		}).WithTimeout(5 * time.Second).Should(Succeed())
	})
})

func setupFakes(kubeclient kubernetes.Interface) {
	status_updater.InClusterConfigFn = func() (*rest.Config, error) {
		return nil, nil
	}
	status_updater.KubeClientFn = func(c *rest.Config) kubernetes.Interface {
		return kubeclient
	}
}

func setupConfig(kubeclient kubernetes.Interface) {
	setupEnvs()
}

func setupEnvs() {
	os.Setenv("KUBERNETES_SERVICE_HOST", "fake-host")
	os.Setenv("KUBERNETES_SERVICE_PORT", "fake-port")
	os.Setenv("TOPOLOGY_CM_NAME", "fake-cm-name")
	os.Setenv("TOPOLOGY_CM_NAMESPACE", "fake-cm-namespace")
}
