[Ticket](https://runai.atlassian.net/browse/RUN-6464)

# Node Autoscale Design

## Motivation
Topology CM *must* have a section for each fake GPU node in order for it to run.
This section can be created manually, or automatically by the status-updater upon restart.
This means that when nodes are added, we need to restart the status-updater so it can refresh the topology CM.
We want this automatic flow to happen continuosly and not just upon restart.

## Action Items
- Upon restart, cleanup the topology from nodes that doesn't exist
- Add node controller to update the topology's nodes continuosly
  - Filter in nodes with label `nvidia.com/gpu.deploy.dcgm-exporter` and `nvidia.com/gpu.deploy.device-plugin` set to true