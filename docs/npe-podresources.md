# Fake podresources for KAI NUMA placement (`npe`)

The status-exporter can serve a **fake kubelet podresources gRPC socket** and a matching
**sysfs `cpulist` tree** on each fake-GPU node, so KAI-Scheduler's numa-placement-exporter
(`npe`) observes fake GPU/CPU/memory NUMA placement on real nodes — without real
multi-socket hardware.

This is off by default.

## What it does

For every pod that holds fake GPUs on a node, the status-exporter synthesizes a podresources
`List` response and a `cpulist` topology from the node's topology ConfigMap and the pool's
`numa` block:

- **GPUs** are grouped by NUMA zone (one `ContainerDevices` entry per zone, each tagged with a
  single NUMA node).
- **CPU and memory** are charged to those zones in proportion to the pod's per-zone GPU count,
  from the pod's resource *requests*.
- The **sysfs `cpulist`** tree (`devices/system/node/node<N>/cpulist`) lets `npe` resolve
  CPU-id → NUMA node, since the podresources API carries CPU ids without topology.

`npe` reads these, writes the `kai.scheduler/numa-placement-observed` annotation on each pod,
and the KAI scheduler's `numa` plugin (running with its default `reconstructAvailable=true`)
reconstructs per-zone availability from those observed placements. Live `NodeResourceTopology`
`available` is therefore **not** required on this path.

## Enable it

```yaml
statusExporter:
  podResources:
    enabled: true
topology:
  nodePools:
    default:
      gpu: { backend: fake }
      numa: { zones: 2 }        # declare the fake NUMA layout
```

When enabled, the status-exporter serves on FGO-owned host paths (never the kubelet's own,
to avoid colliding with the real kubelet):

- podresources socket: `/var/lib/fake-gpu-operator/pod-resources/kubelet.sock`
- sysfs root: `/var/lib/fake-gpu-operator/sys` (`.../devices/system/node/node<N>/cpulist`)

Both are backed by `hostPath` volumes (`DirectoryOrCreate`) on the node.

## Point `npe` at the FGO paths

Configure the KAI numa-placement-exporter to read the FGO socket and sysfs tree instead of the
real kubelet's, and confine it to the fake-GPU nodes:

- `--podresources-socket=/var/lib/fake-gpu-operator/pod-resources/kubelet.sock`
- `--sysfs-root=/host/fake-sys` — with a `hostPath` mount of `/var/lib/fake-gpu-operator/sys`
  at `/host/fake-sys` in the `npe` container
- a `nodeSelector` scoping `npe` to the fake-GPU nodes

The `npe` binary accepts arbitrary values for both flags. Make sure the KAI scheduler's `numa`
plugin keeps its default `reconstructAvailable=true` so it reconstructs per-zone availability
from `npe`'s observed placements.

## Dependency and interim option

The recommended integration relies on the KAI operator exposing the `npe` socket / sysfs-root /
volume-mount settings so the operator-managed `npe` DaemonSet can be pointed at the FGO paths.
Until that configuration surface lands, run a `npe` DaemonSet directly (outside the operator's
managed one) with the flags and `hostPath` mounts above, scoped by `nodeSelector` to the
fake-GPU nodes.

## Scope

Covered: whole-GPU allocations (the pod named by each GPU's `allocatedBy`), synthesized CPU and
memory placement, and the `cpulist` tree.

Not covered: non-GPU pods' CPU/memory, the QoS/Guaranteed-integer CPU-Manager gate, and the
KWOK path (a single central pod cannot serve one kubelet socket per virtual node — this feature
targets real nodes where pods execute).

Shared/fractional GPUs (the reservation model, where a GPU's `allocatedBy` names a
reservation pod in the reservation namespace rather than the workload) are also not covered:
the synthesized entry would be named after the reservation pod, so `npe` would annotate that
pod and the real workload pod would get no placement. Use whole-GPU (dedicated/DRA)
allocations, where `allocatedBy` is the workload pod.
