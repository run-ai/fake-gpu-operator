# Mock backend

The mock backend layers NVIDIA's [`nvml-mock`](https://github.com/NVIDIA/k8s-test-infra/tree/main/deployments/nvml-mock) onto fake-gpu-operator's per-pool topology. It gives a node a **real `libnvidia-ml.so`** (so `nvidia-smi` runs, the upstream NVIDIA device-plugin and DRA driver enumerate devices through NVML) while the GPUs themselves are still synthetic.

This is heavier than the `fake` backend — it writes files to host paths and runs privileged containers — so it's documented separately.

## Enabling

Per pool, in the topology:

```yaml
topology:
  nodePools:
    my-mock-pool:
      gpu:
        backend: mock
        profile: a100   # or h100, b200, gb200, l40s, t4
```

A consumer subchart must also be on so the per-pool ServiceAccount renders:

```yaml
gpuOperator:    { enabled: true }   # device-plugin + GFD + validator (heavyweight)
# OR
nvidiaDraDriver: { enabled: true }   # DRA-only (lighter)
```

Both can be on for parallel device-plugin + DRA paths on different pools.

### Defaults the chart applies for mock-pool use

The chart's `values.yaml` ships with mock-friendly defaults under the
`gpu-operator:` block — you don't normally need to override them:

- `gpu-operator.toolkit.env: [CREATE_DEVICE_NODES=none]` — when you flip
  `gpu-operator.toolkit.enabled: true`, the installer skips real-NVML
  device enumeration that would otherwise fail with "no NVIDIA devices
  found." Without this, the toolkit DS crashes and the `nvidia` runtime
  never gets registered in containerd.
- `gpu-operator.gfd.enabled: false` — FGO's status-exporter writes the
  `nvidia.com/gpu.product/count/memory` node labels that GFD would
  duplicate, and GFD's pod can't load mock NVML in any case. Leaving GFD
  disabled removes a cosmetic CrashLoopBackOff.

To use the mock backend on a pool, flip these two top-level toggles:

```yaml
gpuOperator: { enabled: true }
gpu-operator:
  toolkit: { enabled: true }
```

### Known limitation — `ClusterPolicy` reports `NotReady`

The `gpu-operator` validator DaemonSet's `toolkit-validation` init container
hardcodes `exec nvidia-smi` in a container that has no access to the mock
NVML stack. It cannot succeed on a mock-NVML node. `ClusterPolicy` therefore
stays `NotReady` with `state-operator-validation`.

**This is cosmetic** — workloads requesting `nvidia.com/gpu` schedule
normally, mock devices are injected into containers via CDI, and `nvidia-smi`
inside workload pods reports the mock GPUs correctly. The `NotReady` status
is operator-side health reporting, not a capability gate.

A future upstream `gpu-operator` change to expose `validator.enabled: false`
would resolve this. The validator state is currently hardcoded to always
reconcile (`controllers/state_manager.go:state-operator-validation` returns
`true` unconditionally).

## Host side effects

Each nvml-mock pod writes mock libraries + a CDI spec under `/var/lib/nvml-mock/` and `/var/run/cdi/` on its node. Char devices are created under that tree — **the host's `/dev` is not touched.** Full file list is in [upstream's setup.sh](https://github.com/NVIDIA/k8s-test-infra/blob/main/deployments/nvml-mock/scripts/setup.sh).

The DaemonSet's `preStop` hook (`cleanup.sh`) removes everything on graceful pod termination. **Don't force-delete with `--grace-period=0`** — that skips the hook and leaves artifacts behind that need manual `rm -rf` to recover.

## Coexistence with the `fake` backend

A node is in **one** pool at a time — either `fake` or `mock`, not both. The two backends coexist at the cluster level: fake pools have FGO's device-plugin advertising synthetic GPUs; mock pools have nvml-mock laying down host files for the upstream gpu-operator / DRA driver to consume. Workloads request `nvidia.com/gpu` the same way regardless.

## Known limitations

- **Switching a node's pool at runtime** doesn't re-reconcile its FGO-applied labels — status-updater's node controller only watches Add/Delete, not Update. Drain + rejoin (or delete the device-plugin pod) to force a clean transition.
- **`--reuse-values` upgrade fails** for users whose stored values predate new top-level keys — [#195 / RUN-39195](https://github.com/run-ai/fake-gpu-operator/issues/195). Use `helm upgrade -f values.yaml` instead.
- **In-pod `nvidia-smi` reports the compiled-in default model** (A100) regardless of pool profile. The mock library inside DRA- and device-plugin-allocated pods can't auto-locate the per-pool config. See `docs/RUN-38195-nvml-mock-failure-explainer.html`.

## When to use which

| Backend | Use when |
|---|---|
| `fake` | Scale testing, KWOK-friendly, no host mutation. **Default — start here.** |
| `mock` | Workloads call into real NVML (CUDA device discovery, `nvidia-smi`, etc.). Requires nodes with mutable host filesystems. |
