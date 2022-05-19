package status_updater_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	main "github.com/run-ai/gpu-mock-stack/cmd/status-updater"
)

func TestStatusUpdater(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "StatusUpdater Suite")
}

var _ = Describe("StatusUpdater", func() {
	BeforeAll(func() {
		main.InClusterConfigFn = func() (*rest.Config, error) {
			return nil, nil
		}

		main.KubeClientFn = func(config *rest.Config) kubernetes.Interface {
			return fake.NewSimpleClientset()
		}
	})
})
