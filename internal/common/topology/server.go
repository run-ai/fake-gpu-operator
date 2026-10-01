package topology

import "fmt"

// DefaultTopologyServerNamespace is used when the install namespace is unknown.
const DefaultTopologyServerNamespace = "gpu-operator"

// NodeTopologyServerURL returns the topology-server endpoint for a node, where
// namespace is the namespace the chart is installed in.
func NodeTopologyServerURL(namespace, nodeName string) string {
	if namespace == "" {
		namespace = DefaultTopologyServerNamespace
	}
	return fmt.Sprintf("http://topology-server.%s/topology/nodes/%s", namespace, nodeName)
}
