package node

import (
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	v1 "k8s.io/api/core/v1"
)

func isFakeNode(node *v1.Node) bool {
	return node != nil && node.Annotations[constants.AnnotationKwokNode] == "fake"
}
