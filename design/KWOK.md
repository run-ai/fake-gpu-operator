# Support running on KWOK cluster

## Ticket
https://runai.atlassian.net/browse/RUN-16744

## Motivation
KWOK allows running fake pods on fake nodes.
We want to support running on KWOK cluster to reduce the cost of running scale tests on real nodes.

## Assumptions
- Pods that run on fake nodes are fake pods (their containers won't run)

## Limitations
- GPU nodes should either be all real or all fake (mixing of real and fake GPU nodes on the same cluster is not supported)
- Since we start faking the Node Exporter, Node Exporter logic won't be tested, and logic changes to it should be applied to the Fake GPU Operator.
- MIG won't be supported (support will be added later on if needed)
- `nvidia-smi` won't be supported.

## Requirements
- Metrics should be exposed the same way as on real nodes

## Gaps
- Status Exporter
  - Currently deployed as a DaemonSet (unable to run on fake nodes).
  - RunAI's Node Exporter is also deployed as a DaemonSet. Therefore, since either need to deploy it as a Deployment, or fake its behavior (by exporting its metrics directly).
- Device Plugin
  - Currently deployed as a DaemonSet. It won't be able to run on fake nodes and therefore should be configurable to be deployed as a Deployment instead, or stop using it and edit nodes manually.

## Design
- [ ] Create a `status-exporter` deployment that will be a single monolithic service that will handle all exportation logic when GPU nodes are fake. This service will handle the following:
  - [ ] Metrics
    - [ ] Export the same as today, with the following label enrichments (<pod> refers to the dcgm-exporter fake pod):
      - [ ] `container="nvidia-dcgm-exporter"`
      - [ ] `instance="<pod-ip>:9400"`
      - [ ] `job="nvidia-dcgm-exporter"`
      - [ ] `pod="<pod-name>"`
      - [ ] `service="nvidia-dcgm-exporter"`
  - [ ] FileSystem
    - [ ] Instead of exporting to the FileSystem and be read by the Node Exporter, we'll export Node Exporter's metrics directly. The following metrics would be exported:
      - [ ] name: `runai_pod_gpu_utilization`, labels: `pod_uuid`, `gpu`
      - [ ] name: `runai_pod_gpu_memory_used_bytes`, labels: `pod_uuid`, `gpu`
  - [ ] Labels
    - [ ] Export the same as today.
- [ ] Add a ServiceMonitor for the new service, and set `honorLabels: true` on it (so we can fake multiple exporters).