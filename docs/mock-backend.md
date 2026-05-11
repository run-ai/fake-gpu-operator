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
