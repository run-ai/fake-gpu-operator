package handle_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestHandle(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Handle Suite")
}
