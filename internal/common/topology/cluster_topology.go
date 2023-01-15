package topology

func (ct *Cluster) GetNodeTopology(nodeName string) (*Node, error) {
	if ct.Nodes == nil {
		return nil, ErrNoNodes
	}

	nodeTopology, ok := ct.Nodes[nodeName]
	if !ok {
		return nil, ErrNoNode
	}

	return &nodeTopology, nil
}
