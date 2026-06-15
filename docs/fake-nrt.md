# NUMA topology (NodeResourceTopology)

fake-gpu-operator can publish a [`NodeResourceTopology`](https://github.com/k8stopologyawareschedwg/noderesourcetopology-api) (NRT) custom resource per fake-GPU node. NRT is what NUMA-aware schedulers (e.g. KAI-Scheduler) read to place GPU workloads across NUMA zones — they **filter** on a zone's `available` resources and **score** on the inter-zone `costs` (distances).

This lets you test NUMA-aware GPU scheduling on KWOK / KIND / EKS **without real multi-socket hardware**: the topology is declared in config and the status-updater fabricates the NRT, rather than relying on a real kubelet reading `/sys`.

## Enabling

NRT publishing is **off by default**. Turn it on with `statusUpdater.nodeResourceTopology.enabled`, then add a `numa` block to each pool you want NRTs for (pools without one are unaffected):

```yaml
statusUpdater:
  nodeResourceTopology:
    enabled: true              # master switch (default false)
    installCRD: true           # install the NRT CRD; set false if it already exists

topology:
  nodePools:
    default:
      gpu:
        backend: fake          # works with fake or mock
        profile: a100          # the profile decides the pool's GPU count (a100 = 8)
      numa:
        zones: 2               # REQUIRED — number of NUMA zones
        distances:
          self: 10
          remote: 21
```

When enabled, the chart installs the NRT `CustomResourceDefinition` (annotated `helm.sh/resource-policy: keep`, so uninstalling fake-gpu-operator never deletes it or anyone's NRTs) and grants the status-updater RBAC for it. If the CRD already exists — NFD's topology-updater or KAI-Scheduler commonly install it — set `installCRD: false` so Helm doesn't try to re-create a CRD it doesn't own.

## What gets published

For the pool above, a node `kwok-numa-0` with `32` allocatable CPU and `256Gi` memory yields:

```yaml
apiVersion: topology.node.k8s.io/v1alpha2
kind: NodeResourceTopology
metadata:
  name: kwok-numa-0          # one NRT per node, named after the node (cluster-scoped)
attributes:
- {name: topologyManagerPolicy, value: single-numa-node}
- {name: topologyManagerScope,  value: container}
topologyPolicies: [SingleNUMANodeContainerLevel]   # deprecated mirror of the attributes
zones:
- name: node-0
  type: Node
  costs: [{name: node-0, value: 10}, {name: node-1, value: 21}]
  resources:
  - {name: nvidia.com/gpu, capacity: "4", allocatable: "4", available: "4"}
  - {name: cpu,            capacity: "16", allocatable: "16", available: "16"}
  - {name: memory,         capacity: 128Gi, allocatable: 128Gi, available: 128Gi}
- name: node-1
  type: Node
  costs: [{name: node-0, value: 21}, {name: node-1, value: 10}]
  resources:
  - {name: nvidia.com/gpu, capacity: "4", allocatable: "4", available: "4"}
  - {name: cpu,            capacity: "16", allocatable: "16", available: "16"}
  - {name: memory,         capacity: 128Gi, allocatable: 128Gi, available: 128Gi}
```

The pool's 8 GPUs split evenly across the 2 zones, and node-allocatable CPU/memory are split across the zones.

## Configuration reference

All keys live under `nodePools.<pool>.numa`:

| key | required | default | example | meaning |
|---|---|---|---|---|
| `zones` | **yes** | — | `2` | Number of NUMA zones. Set to `0`/omit the block to disable publishing for the pool. |
| `gpusPerZone` | no | even split | `[6, 2]` | Explicit per-zone GPU counts. Must have one entry per zone and sum to the pool's GPU count. |
| `topologyManagerPolicy` | no | `single-numa-node` | `restricted` | Value of the `topologyManagerPolicy` attribute. |
| `topologyManagerScope` | no | `container` | `pod` | Value of the `topologyManagerScope` attribute. |
| `cpuPerZone` | no | node allocatable CPU ÷ zones | `"4"` | Per-zone CPU quantity. |
| `memPerZone` | no | node allocatable memory ÷ zones | `64Gi` | Per-zone memory quantity. |
| `distances` | no | `{self: 10, remote: 21}` | `{self: 10, remote: 30}` | Zone distance costs; `self` for a zone to itself, `remote` for any other zone. |

The pool's **GPU count** comes from its `gpu` profile (or overrides) — the same value FGO advertises as `nvidia.com/gpu`. If `cpuPerZone`/`memPerZone` are unset and the node has no allocatable CPU/memory, those resources are simply omitted from the zones (GPU-only zones).

## Notes and caveats

- **Static `available`.** Each zone reports `available == allocatable == capacity`. The published topology is fixed at node-add time; `available` does not yet decrease as pods are scheduled.
- **Don't run NFD's topology-updater on the same nodes.** NFD's [node-feature-discovery](https://github.com/kubernetes-sigs/node-feature-discovery) topology-updater writes the same per-node NRT (same CR name) from real `/sys`, so the two would clobber each other. Scope NFD away from the nodes this manages. On KWOK there is no NFD, so there is no conflict.
- **One NRT per node, cluster-scoped**, named after the node. It is removed when the node is deleted.
