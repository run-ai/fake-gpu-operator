# Support running on KWOK cluster

## Ticket
https://runai.atlassian.net/browse/RUN-16744

## Motivation
The KWOK cluster enables running fake pods on fake nodes, offering an opportunity to reduce the cost of running scale tests on real nodes. 
This initiative aims to extend support for our system to operate seamlessly within the KWOK cluster environment, so we can reduce the cost of running scale tests on real nodes.

## Assumptions
- Pods running on fake nodes are considered fake pods. These pods' containers will not execute.

## Requirements
- Ensure labels and metrics are exposed in the same manner as on real nodes.

## Gaps
- Status Exporter
  - The current deployment as a DaemonSet is incompatible with fake nodes.
- RunAI's Node Exporter
  - The current deployment as a DaemonSet is incompatible with fake nodes.
- Device Plugin
  - The current deployment as a DaemonSet is incompatible with fake nodes. We might want to not supoprt it on fake nodes and require manual node resources update.

## Design
In order to solve the above gaps, we'll create a service that will replace the current exporters and will be enabled only when running on the KWOK cluster.
This service will live alongside the current exporters, since we still want to mimic the real environment as much as possible (e.g. for exporting kube-state-metrics on the exporter pods).

The metrics that will be exported from the new service will need to contain the exact labels as the current exporters, meaning we'll need to override the labels that prometheus sets on the metrics based on their origin.
In order to do that, we'll need to set `honorLabels: true` on the ServiceMonitor for the new service.

## Action Items
- [ ] Implement a single monolithic service named `centralized-status-exporter` to handle all exportation logic when GPU nodes are fake. This service will be disabled by default, and will be manually enabled when running on kwok cluster. This service will encompass the following:
  - [ ] Metrics
    - [ ] Export the same as today, with the following label enrichments (<pod> refers to the dcgm-exporter fake pod):
      - [ ] `container="nvidia-dcgm-exporter"`
      - [ ] `instance="<pod-ip>:9400"`
      - [ ] `job="nvidia-dcgm-exporter"`
      - [ ] `pod="<pod-name>"`
      - [ ] `service="nvidia-dcgm-exporter"`
  - [ ] FileSystem
    - [ ] Directly export Node Exporter's metrics instead of exporting to the FileSystem, including:
      - [ ] `runai_pod_gpu_utilization` with labels `pod_uuid` and `gpu`
      - [ ] `runai_pod_gpu_memory_used_bytes` with labels `pod_uuid` and `gpu`
  - [ ] Labels
    - [ ] Ensure consistent label exportation.
  - [ ] Node Devices
    - [ ] Ensure consistent device exportation.
- [ ] Add a ServiceMonitor for the new service, and set `honorLabels: true` on it (so we can fake multiple exporters).

## Limitations
- GPU nodes must be either all real or all fake. Mixing real and fake GPU nodes within the same cluster is not supported.
- Faking the Node Exporter implies that Node Exporter logic won't be tested. Any logic changes to it should be applied to the Fake GPU Operator.
- Multi-Instance GPU (MIG) configuration is not supported initially, but support can be added later if necessary.
- `nvidia-smi` won't be supported.