package labels

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
)

// invalidLabelChars matches any character not allowed in a K8s label value.
// Valid chars: [a-zA-Z0-9._-]
var invalidLabelChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// BuildNodeLabels creates the standard node labels from a topology
func BuildNodeLabels(nodeTopology *topology.NodeTopology) map[string]string {
	return map[string]string{
		"nvidia.com/gpu.memory":   strconv.Itoa(nodeTopology.GpuMemory),
		"nvidia.com/gpu.product":  sanitizeLabelValue(nodeTopology.GpuProduct),
		"nvidia.com/mig.strategy": nodeTopology.MigStrategy,
		"nvidia.com/gpu.count":    strconv.Itoa(len(nodeTopology.Gpus)),
		"nvidia.com/gpu.present":  "true",
		"run.ai/fake.gpu":         "true",
	}
}

// sanitizeLabelValue replaces characters invalid in Kubernetes label values
// with dashes and enforces the 63-character limit, matching NVIDIA GFD conventions.
// Valid label values must match [a-zA-Z0-9._-]{0,63} and start/end with alphanumeric.
func sanitizeLabelValue(s string) string {
	s = invalidLabelChars.ReplaceAllString(s, "-")
	if len(s) > 63 {
		s = s[:63]
	}
	// Trim leading/trailing non-alphanumeric chars (K8s requirement)
	s = strings.TrimFunc(s, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	})
	return s
}
