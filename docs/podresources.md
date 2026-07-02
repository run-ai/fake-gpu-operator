# Fake kubelet podresources + sysfs for NUMA placement

The status-exporter can serve a **fake kubelet podresources gRPC socket** and a matching
**sysfs `cpulist` tree** on each fake-GPU node, so any tool that consumes the kubelet
[podresources API](https://kubernetes.io/docs/concepts/cluster-administration/device-plugins/#monitoring-device-plugin-resources)
observes fake GPU/CPU/memory NUMA placement on real nodes — without real multi-socket
hardware. KAI-Scheduler's numa-placement-exporter (`npe`) is one such consumer; see the
example below.

This is off by default.

## What it does

For every pod that holds fake GPUs on a node, the status-exporter synthesizes a podresources
`List` response and a `cpulist` topology from the node's topology ConfigMap and the pool's
`numa` block:

- **GPUs** are grouped by NUMA zone (one `ContainerDevices` entry per zone, each tagged with a
  single NUMA node).
- **CPU and memory** are charged to those zones in proportion to the pod's per-zone GPU count,
  from the pod's resource *requests*.
- The **sysfs `cpulist`** tree (`devices/system/node/node<N>/cpulist`) lets a consumer resolve
  CPU-id → NUMA node, since the podresources API carries CPU ids without topology.

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

## Point a consumer at the FGO paths

Any podresources client points at the FGO socket and sysfs tree instead of the real kubelet's,
and is scoped to the fake-GPU nodes (e.g. via a `nodeSelector`):

- podresources socket: `/var/lib/fake-gpu-operator/pod-resources/kubelet.sock`
- sysfs root: `/var/lib/fake-gpu-operator/sys` — mount it into the consumer container with a
  `hostPath` volume and point the consumer's sysfs-root at the mount path

## Example: KAI numa-placement-exporter (`npe`)

Configure `npe` to read the FGO socket and sysfs tree, and confine it to the fake-GPU nodes:

- `--podresources-socket=/var/lib/fake-gpu-operator/pod-resources/kubelet.sock`
- `--sysfs-root=/host/fake-sys` — with a `hostPath` mount of `/var/lib/fake-gpu-operator/sys`
  at `/host/fake-sys` in the `npe` container
- a `nodeSelector` scoping `npe` to the fake-GPU nodes

The `npe` binary accepts arbitrary values for both flags. `npe` writes the
`kai.scheduler/numa-placement-observed` annotation on each pod, and the KAI scheduler's `numa`
plugin (running with its default `reconstructAvailable=true`) reconstructs per-zone
availability from those observed placements. Live `NodeResourceTopology` `available` is
therefore **not** required on this path.

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
the synthesized entry would be named after the reservation pod, so a consumer would attribute
placement to that pod and the real workload pod would get none. Use whole-GPU (dedicated/DRA)
allocations, where `allocatedBy` is the workload pod.
