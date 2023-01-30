# MIG Metrics Design

## Running prerequisites
- Node labels:
  - `nvidia.com/gpu.product` set to a MIG supported GPU (e.g. `NVIDIA-A100-SXM4-40GB`)
  - `node-role.kubernetes.io/runai-dynamic-mig` set to true
  - `node-role.kubernetes.io/runai-mig-enabled` set to true

## Requirements
Nodes with the label `node-role.kubernetes.io/runai-dynamic-mig` set to true will export the following metrics according to the samples:
- `DCGM_FI_DEV_FB_USED`
- `DCGM_FI_DEV_FB_FREE`

`DCGM_FI_DEV_GPU_UTIL` metric should *not* be exported.

Exported metrics will contain the following labels:
- gpu
- UUID
- device
- modelName
- Hostname
- container
- namespace
- pod
- GPU_I_PROFILE
- GPU_I_ID
- DCGM_FI_DRIVER_VERSION
(The last 3 do not exist on non-mig metrics)

## Real Metric Inspection

### Artifacts
Dynamic MIG samples are stored under `samples/mig` directory.

### Conclusions
- Metrics are exported per GPU Instance and don't include allocating container/pod information
- The scheduler exports allocation info on the pods `runai-mig-device` label, containing gpuAmount, position, instance name and GPU Index

## High level design
- The topology configmap should look like so:
```yml
    mig-strategy: mixed
    nodes:
      runai-control-plane:
        gpu-count: 1
        gpu-memory: 40000
        gpu-product: NVIDIA-A100-SXM4-40GB
        is-dynamic-mig-enabled: true
        gpus:
        - id: GPU-91350851-5c96-531a-ad25-0034a5bf120f
          mig-devices:
          - id: MIG-28810d46-0180-5139-a975-020bdc7f9cb1
            name: 1g.5gb
            status:
              allocated-by:
                namespace: runai-pa
                pod: job-0-0-0
                container: job-0
              pod-gpu-usage-status:
                e60f84df-bbb4-444d-b25f-77be1d67a068:
                  utilization:
                    # Not used at the moment (nvidia doesn't support utilization metrics on MIG instances)
                    min: 80
                    max: 100
                  fb-used: 1144
                  is-inference-pod: false
```
- Only Metrics exporter will support dynamic MIG

## Low level design
### Topology ConfigMap
- The topology configmap will contain the following changes:
  - Add `is-dynamic-mig-enabled` field on each node
  - Add `mig-devices` as optional parameter on the `gpus` section, which will contain `GpuDetails` with the addition of `name` field
  - Make `Status` field of `GpuDetails` optional
### Status Updater
The Status Updater should update the topology ConfigMap with details about node's dynamic MIG enablement and MIG GPUs devices.
- Node controller will set `is-dynamic-mig-enabled` on nodes based on the `node-role.kubernetes.io/runai-dynamic-mig` and `node-role.kubernetes.io/runai-mig-enabled` fields
- Node controller will set `mig-devices` on nodes based on the `run.ai/mig.config` and `run.ai/mig-mapping` fields
  - Upon `run.ai/mig.config` change, `mig-devices` will be created without an id field, which will be filled once the `run.ai/mig-mapping` label is updated.
  - Upon `run.ai/mig-mapping` change, `mig-devices` will be created and updated with the id from the `run.ai/mig-mapping` label and device name from the `run.ai/mig.config` label, associated by the position of the device (GpuIdx + position)
- Pod Controller will find the position to set the allocation status of the `mig-device` by the pod's `runai-mig-device` label
### Status Exporter
- Once the metrics exporter encounters a node with `is-dynamic-mig-enabled` set to true, it will:
  - Unset the following labels:
    - container
    - namespace
    - pod
  - Set `GPU_I_PROFILE` using the `name` field of the `mig-device`
  - Conclude the GPU Instance Profile ID and set it on the `GPU_I_ID` label
  - Set `DCGM_FI_DRIVER_VERSION` label with a fixed value of `520.56.06`
  - Publish GPU memory usage as usual