# GPU Mock design

- [GPU Mock design](#gpu-mock-design)
- [Phase 1](#phase-1)
  - [Mock requirements](#mock-requirements)
  - [Config Structure](#config-structure)
  - [Components](#components)
  - [Squence Diagram](#squence-diagram)
- [Phase 2](#phase-2)

# Phase 1

## Mock requirements

- Node resources
  - `nvidia.com/gpu`
- Node labels
  - `nvidia.com/gpu.memory`
  - `nvidia.com/gpu.count`
  - `nvidia.com/mig.strategy`
  - `nvidia.com/gpu.product`
- Metrics
  - `DCGM_FI_DEV_GPU_UTIL` (gpu, UUID, device, modelName, Hostname, container, namespace, pod)
  - `DCGM_FI_DEV_FB_USED`
  - `DCGM_FI_DEV_FB_FREE`

## Config Structure

```yml
mig-strategy: mixed
nodes:
  node-a:
    gpu-memory: 11441
    gpu-product: Tesla-K80
    gpus:
      - id: GPU-8fc73fbd-91f0-697e-57ef-fc978bd54af1
        # This section is filled by the status updater
        metrics:
          metadata:
            namespace: runai-project-a
            pod: pod
            container: container
          utilization: 10
          fb-used: 5000
      - id: GPU-kdiue726-57ef-91f0-837f-qjk49djklsk8
        # This section is filled by the status updater
        metrics:
          metadata:
            namespace: runai-project-a
            pod: pod
            container: container
          utilization: 100
          fb-used: 10000
```

## Components

Device Plugin:

- Exposes GPUs based on the ConfigMap
- On Allocate call, does nothing

Status Updater:

- Watch for running pods that requested GPUs
- Update the ConfigMap accordingly (pod up == 100% utilization on arbitrary GPU)

DCGM Exporter:

- Exports metrics based on the ConfigMap

GFD:

- Exports metrics based on the ConfigMap

## Squence Diagram

```https://sequencediagram.org
title Mock GPU workload flow

Kubelet -> Mock device plugin: Allocate(id=0)
Mock device plugin -> Kubelet: AllocateResponse{}

Status Updater -> API Server: Update ConfigMap

Mock DCGM Exporter -> Mock DCGM Exporter: Export Node Status
Mock GFD -> Mock GFD: Export Node Status
```

# Phase 2

- [x] Status-exporter -> Daemonset
- [ ] DCGM Exporter - Export metrics for the exact container
- [ ] Mimick failures in the device plugins
- [ ] Hot config change (affect without restart)
- [ ] Support multi-container pods
- [ ] Client - Add utilization options
  ```json
  {
      {
          time: "1h"
          utilization: 80
      },
      {
          time: "1h"
          utilization: 0
      }
      repeat: 5
  }
  ```
