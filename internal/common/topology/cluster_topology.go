package topology

func (ct *ClusterTopology) GetNodeTopology(nodeName string) (*NodeTopology, error) {
	if ct.Nodes == nil {
		return nil, ErrNoNodes
	}

	nodeTopology, ok := ct.Nodes[nodeName]
	if !ok {
		return nil, ErrNoNode
	}

	return &nodeTopology, nil
}
