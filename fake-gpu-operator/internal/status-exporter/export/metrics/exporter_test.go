package metrics

import "testing"

func TestGenerateFakeHostname(t *testing.T) {
	cases := []struct {
		nodeName string
		expected string
	}{
		{
			nodeName: "",
			expected: "nvidia-dcgm-exporter-da39a3",
		},
		{
			nodeName: "node-1",
			expected: "nvidia-dcgm-exporter-b36828",
		},
		{
			nodeName: "node-1-2-3",
			expected: "nvidia-dcgm-exporter-a0d194",
		},
	}

	for _, c := range cases {
		actual := generateFakeHostname(c.nodeName)
		if actual != c.expected {
			t.Errorf("generateFakeHostname(%s) = %s, expected %s", c.nodeName, actual, c.expected)
		}
	}
}
