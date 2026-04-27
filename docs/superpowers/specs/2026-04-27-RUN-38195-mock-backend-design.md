# Mock Backend Support — Phase 5 Design

**Jira:** [RUN-38195](https://runai.atlassian.net/browse/RUN-38195)
**Date:** 2026-04-27
**Status:** Draft
**Supersedes:** the now-reverted Phase 4 component controller (RUN-38194), whose real customer turned out to be mock pools, not fake pools

## Goal

Support `backend: mock` node pools end-to-end: a real Linux GPU node, scoped to a mock pool, runs the upstream NVIDIA GPU Operator stack against a mocked NVML driver layer (nvml-mock). Workloads requesting `nvidia.com/gpu` schedule on those nodes and run as if the cluster had real GPUs.

This spec lands as a single PR. The three pieces below are operationally coupled — each is non-functional without the others — so splitting them across PRs would just produce dead intermediate states.

## Architecture

Three layers stack per mock-pool node:

| Layer | What it provides | Owner | Where it lives |
|---|---|---|---|
| **L1: nvml-mock DaemonSet + ConfigMap (per pool)** | Lays down a fake `libnvidia-ml.so`, fake `/dev/nvidia*` device files, profile config at `/var/lib/nvml-mock/driver/` | Status-updater controller (new) | `internal/status-updater/controllers/mock/` |
| **L2: GPU Operator** | Real device-plugin / GFD / DCGM-exporter, configured to read NVML through L1's mocked driver root | Helm subchart | `deploy/fake-gpu-operator/Chart.yaml`, `values.yaml` |
| **L3: Node labeling** | `nvidia.com/gpu.deploy.*` labels on mock-pool nodes that GPU Operator's components use as nodeSelectors | Status-updater NodeController (existing, narrowed) | `internal/status-updater/handlers/node/labels.go` |

End-to-end flow when a user enables mock support:

```
helm install with gpuOperator.enabled=true
        │
        ├─ Chart renders: GPU Operator subchart (L2) installs
        ├─ Status-updater pod starts with MOCK_CONTROLLER_ENABLED=true
        │
        ▼
Status-updater                    Topology CM (with backend: mock pools)
  ├─ NodeController (L3): real Linux nodes labeled with pool key get
  │   nvidia.com/gpu.deploy.* labels — GPU Operator components target them
  │
  └─ MockController (L1): per mock pool, builds nvml-mock-{pool} DaemonSet
      + nvml-mock-{pool} ConfigMap; reconciles on topology CM changes
        │
        ▼
On a mock-pool node:
  nvml-mock DaemonSet writes /var/lib/nvml-mock/driver/{libnvidia-ml.so, devices, config.yaml}
  GPU Operator's device-plugin/GFD/DCGM-exporter call NVML → see mock GPUs
  Pods requesting nvidia.com/gpu schedule and run
```

## Piece B — GPU Operator subchart

### Chart.yaml dependency

```yaml
dependencies:
  - name: gpu-operator
    version: "26.3.1"
    repository: https://helm.ngc.nvidia.com/nvidia
    condition: gpuOperator.enabled
```

Pinned to v26.3.1 (latest as of 2026-04-18), bumping from the previously-stale 24.9.0. No alias — the subchart's values key is the literal `gpu-operator:` (hyphenated). This is distinct from our existing `gpuOperator:` (camelCase) parent-chart toggle/config block.

### values.yaml structure

```yaml
# Parent-chart-side toggle/config — our domain
gpuOperator:
  enabled: false                # NOTE: flipped from true → false (see migration)
  chartVersion: "26.3.1"        # informational; actual pin lives in Chart.yaml

# Subchart values block — Helm convention, keyed by subchart name.
# Defaults here are the minimum to integrate with nvml-mock. Users add their
# own overrides under this key; Helm merges them on top.
gpu-operator:
  driver:
    enabled: false              # nvml-mock provides the driver; no real install
  toolkit:
    enabled: false              # no real container toolkit either
  nfd:
    enabled: false              # we apply nvidia.com/gpu.deploy.* labels via NodeController
  # Driver-root pointer — the exact subchart value key (driver.driverRoot,
  # validator.driver.env[NVIDIA_DRIVER_ROOT], etc., depending on v26.3.1's
  # schema) is finalized during implementation by reading the upstream chart's
  # values.yaml. Contract: every component that reads a driver root must point
  # at /var/lib/nvml-mock/driver.
```

User override mechanism: users add keys directly under `gpu-operator:` in their own values file. Helm merges them on top of our defaults. We do **not** introduce a `gpuOperator.values:` passthrough block — that pattern is incompatible with how Helm subcharts consume values (parent values.yaml is not templated).

### Files added

- `deploy/fake-gpu-operator/templates/mock/serviceaccount.yaml` — single `ServiceAccount` named `nvml-mock` in the release namespace, gated on `.Values.gpuOperator.enabled`. Every per-pool DaemonSet built by the controller references it. Lifetime tied to the chart, not to any individual pool.

### Files removed

- `deploy/fake-gpu-operator/templates/gpu-operator/deployment.yml` — the placeholder `replicas: 0 / ubuntu:22.04 sleep infinity` Deployment. No replacement; the real subchart now occupies its role.
- `deploy/fake-gpu-operator/templates/gpu-operator/ocp/clusterserviceversion.yaml` is **kept** — gated independently on `environment.openshift`, serves a different purpose.

## Piece C — Mock-pool node labeling

`internal/status-updater/handlers/node/labels.go:labelNode` swaps its predicate from `!isFakeNode(node)` to "node belongs to a pool with `Gpu.Backend == "mock"`":

```go
// before
if !isFakeNode(node) {
    labels[devicePluginLabelKey] = "true"
    labels[draPluginGpuLabelKey] = "true"
    labels[computeDomainDevicePluginLabelKey] = "true"
}

// after
poolName := node.Labels[p.clusterConfig.NodePoolLabelKey]
pool, ok := p.clusterConfig.NodePools[poolName]
if ok && pool.Gpu.Backend == constants.BackendMock {
    labels[devicePluginLabelKey] = "true"
    labels[draPluginGpuLabelKey] = "true"
    labels[computeDomainDevicePluginLabelKey] = "true"
}
```

`dcgmExporterLabelKey` continues to be applied unconditionally — fake-gpu-operator's own kwok-status-exporter consumes it as a nodeSelector. Only the three GPU-Operator-targeting labels become mock-conditional.

`unlabelNode` is unchanged. It removes all four labels regardless of which were set, which is idempotent.

### Behavior matrix

| Node type | Pool's backend | Today | After change |
|---|---|---|---|
| KWOK | `fake` | dcgm-exporter only | dcgm-exporter only — unchanged |
| Real Linux | `mock` | all four | all four — unchanged |
| Real Linux | `fake` | all four | dcgm-exporter only — **behavior change** |
| Real Linux | (no pool match in CM) | all four | dcgm-exporter only — **behavior change** |

The two behavior changes are intentional: real-Linux-in-fake-pool was previously getting GPU Operator's component-placement labels for no reason. The change brings labeling in line with what the labels mean.

### Day-2 dynamics — explicitly out of scope

`NodeController` captures `clusterConfig` once at startup and registers only `AddFunc`/`DeleteFunc` (no `UpdateFunc`, no topology-CM watch). For piece C, this means:

| Scenario | Covered? |
|---|---|
| New real Linux node joins mock pool | ✓ AddFunc fires; labels applied |
| Mock-pool node deleted | ✓ DeleteFunc fires; labels removed |
| Existing node's pool label flips fake↔mock | ✗ requires status-updater rollout |
| Pool's backend in topology CM flips fake↔mock | ✗ requires status-updater rollout |

Documented limitation. Adding `UpdateFunc` and CM-watch is a future enhancement when real workflow demands it — kept out of this spec to minimize blast radius.

## Piece A — Mock controller

### Package layout

A fresh package; does **not** import or revive the deleted Phase 4 controller code.

```
internal/status-updater/controllers/mock/
├── controller.go          # SharedIndexInformer on the topology CM
├── reconciler.go          # readConfig → ComputeDesiredState → diff → apply
├── desired_state.go       # pure: ClusterConfig → []runtime.Object (per mock pool)
├── resources.go           # nvml-mock DaemonSet + ConfigMap builders
├── diff.go                # DaemonSetDiff + ConfigMapDiff
├── profile.go             # profile lookup + override merge → nvml-mock config.yaml
└── *_test.go
```

### Data flow per mock pool

```
pool { profile: "a100", overrides: {gpu_count: 4} }      from topology CM
        │
        ▼
profile.go:
  read CM "gpu-profile-a100" → parse profile.yaml → deep-merge overrides → resolved YAML
        │
        ▼
desired_state.go for this pool:
  ConfigMap   "nvml-mock-{pool}"   data["config.yaml"] = resolved YAML
  DaemonSet   "nvml-mock-{pool}"   nodeSelector pinned to pool, mounts CM
        │
        ▼
diff against actual (filtered by managed-by=fake-gpu-operator) → Create/Update/Delete
```

### Per-pool resource shapes

Replicates upstream nvml-mock's `templates/daemonset.yaml` (~55 lines, reviewed at version bumps). Key fields:

```yaml
DaemonSet  nvml-mock-{pool}
  metadata.labels:
    app.kubernetes.io/managed-by: fake-gpu-operator
    fake-gpu-operator/component:  nvml-mock
    fake-gpu-operator/pool:       {pool}
  spec.template.metadata.annotations:
    fake-gpu-operator/config-hash: <sha of ConfigMap data>
  spec.template.spec:
    nodeSelector:
      <NodePoolLabelKey>: {pool}
    serviceAccountName: nvml-mock          # one shared SA created by the chart
    containers:
      - name: nvml-mock
        image: <ResolveImage(...)>
        imagePullPolicy: <from values>
        securityContext: { privileged: true }
        command: ["/scripts/entrypoint.sh"]
        env:
          - GPU_COUNT      (resolved from profile)
          - DRIVER_VERSION (resolved from profile)
          - NODE_NAME      (downward API: spec.nodeName)
        lifecycle.preStop.exec: ["/scripts/cleanup.sh"]
        volumeMounts:
          /host/var/lib/nvml-mock   → hostPath /var/lib/nvml-mock
          /config                   → ConfigMap nvml-mock-{pool}
          /host/var/run/cdi         → hostPath /var/run/cdi
          /host/run/nvidia          → hostPath /run/nvidia

ConfigMap  nvml-mock-{pool}
  metadata.labels:
    app.kubernetes.io/managed-by: fake-gpu-operator
    fake-gpu-operator/component:  nvml-mock
    fake-gpu-operator/pool:       {pool}
  data:
    config.yaml: |
      <resolved profile YAML — nvml-mock's exact schema, post-overrides>
```

A single chart-templated `ServiceAccount` named `nvml-mock` (gated on `gpuOperator.enabled`) is referenced by every per-pool DaemonSet. Created once by the Helm chart, not the controller.

### Image source

A new field on `ComponentsConfig`:

```go
type ComponentsConfig struct {
    // ...existing fields...
    NvmlMock *ComponentImageConfig `yaml:"nvmlMock,omitempty"`
}
```

Default values in `values.yaml`:

```yaml
nvmlMock:
  image:
    repository: ghcr.io/nvidia/nvml-mock
    tag: "v0.1.0"             # pinned; bumps treated as their own PR
    pullPolicy: IfNotPresent
```

Resolved through the existing `ResolveImage` chain (per-pool image override → global `componentsConfig.imageTag` → fallback to chart values), so per-pool image overrides work the same way they do for fake components.

### Diff strategy

Filter actual state by `app.kubernetes.io/managed-by=fake-gpu-operator` + `fake-gpu-operator/component=nvml-mock`. Two diffs run sequentially:

| Resource | Update predicate |
|---|---|
| DaemonSet | image differs **OR** `fake-gpu-operator/config-hash` annotation differs |
| ConfigMap | `data["config.yaml"]` differs |

DaemonSet update copies `ResourceVersion` and stamps the new config-hash on the pod template — this is what triggers Kubernetes to roll the DaemonSet pods when the ConfigMap content changes.

### Profile resolution + overrides

`profile.go` is a small adapter:

1. Look up ConfigMap `gpu-profile-{name}` in `params.Namespace` (matches RUN-38193's profile CM naming)
2. Parse the `profile.yaml` data key (already in nvml-mock's exact schema — `templates/profiles/builtin.yaml` is auto-generated from `NVIDIA/k8s-test-infra v0.1.0`)
3. Deep-merge `pool.Gpu.Overrides` on top
4. Return serialized YAML for the per-pool ConfigMap

Custom profiles (CMs labelled `fake-gpu-operator/gpu-profile=true` from RUN-38193's `customProfiles`) are looked up the same way — same name pattern, same data key. **Profile-discovery from upstream is explicitly NOT used** — fake-gpu-operator's profile system is the authoritative source.

### Wiring (`app.go`)

The controller is constructed inside `Init` behind a flag, mirroring every other optional controller in the file:

```go
if viper.GetBool(constants.EnvMockControllerEnabled) {
    app.Controllers = append(app.Controllers,
        mockcontroller.NewMockController(app.kubeClient, mockcontroller.ReconcileParams{
            Namespace:        viper.GetString(constants.EnvFakeGpuOperatorNs),
            DefaultRegistry:  viper.GetString(constants.EnvDefaultImageRegistry),
            FallbackTag:      viper.GetString(constants.EnvFallbackImageTag),
            ImagePullPolicy:  pullPolicy,
        }))
}
```

The Helm template plumbs `MOCK_CONTROLLER_ENABLED` from `.Values.gpuOperator.enabled`. One toggle lights up both the subchart and the controller.

### RBAC

Status-updater ClusterRole gains:
- `apps/daemonsets`: `get`, `list`, `watch`, `create`, `update`, `patch`, `delete`
- `""/configmaps`: already has `get`, `list`, `update`, `patch`, `create`, `delete` — no change required
- `""/serviceaccounts`: not needed at runtime (the SA is chart-templated, not controller-created)

No new namespace permissions; resources land in the same namespace as the topology CM.

## Why approach A (raw resources), not Helm SDK

A full pros/cons comparison was performed during brainstorming. Summary:

- **Approach A wins** because it fits the project's existing CM-driven runtime model (every other controller does this), keeps the controller binary small (no Helm SDK / ~50 transitive deps), and keeps RBAC narrow. The known cost is replicating ~55 lines of upstream's DaemonSet template, which is a small surface reviewed at each upstream version bump.
- **Approach B (runtime Helm SDK)** was rejected: it reintroduces the dependency we just removed (Helm SDK in the controller binary), needs broad RBAC for Helm release storage, and adds a second lifecycle layer (release status / rollback semantics) over the K8s resource lifecycle.
- **Static subchart aliases (B3)** was rejected: it caps the pool count, requires `helm upgrade` for Day-2 changes, and breaks the topology-CM-driven runtime model.

## Testing

### Unit tests (no real cluster)

| File | Approximate cases |
|---|---|
| `labels_test.go` | 5 — the behavior matrix in piece C |
| `profile_test.go` | ~8 — no overrides, scalar override, nested override, unknown-key override, missing profile CM, custom profile CM lookup, malformed profile YAML, override type mismatch |
| `resources_test.go` | ~6 — DaemonSet structure, ConfigMap structure, label correctness, image resolution priority, pullPolicy propagation, config-hash annotation present |
| `diff_test.go` | ~10 — DaemonSet (create/update-image/update-config-hash/delete/no-op), ConfigMap (same shape, no update-image case) |
| `reconciler_test.go` | ~6 — empty cluster, pool added, pool removed, image bumped, override changed, multi-pool with mix of backends |

### Helm chart-render tests (no real cluster)

`helm template` assertions:
- `gpuOperator.enabled=false` → no GPU Operator resources, no placeholder Deployment
- `gpuOperator.enabled=true` → expected subchart resources rendered with our mandatory overrides applied
- `gpuOperator.enabled=true` + user override under `gpu-operator:` → user values win where they should, our mandatory overrides preserved
- `environment.openshift=true` path still produces the OCP `ClusterServiceVersion` independently

### End-to-end (real Linux GPU cluster — outside CI)

1. Provision a real Linux node, label with the pool key
2. `helm install` with `gpuOperator.enabled=true` and a mock pool defined in `topology`
3. Verify nvml-mock DaemonSet runs, lays down `/var/lib/nvml-mock/driver/`
4. Verify GPU Operator components reach Running, real device-plugin reports `nvidia.com/gpu` allocatable
5. Schedule a pod requesting `nvidia.com/gpu`; verify it lands on the mock node and `nvidia-smi` inside the container talks to nvml-mock

These tests are documented in a runbook for human validation. The PR description records the user's most recent run.

### Out of test scope

- nvml-mock binary itself (upstream's responsibility)
- GPU Operator components (NVIDIA's responsibility)
- Backward-compat for the placeholder Deployment removal — there's no use case to preserve

## Migration impact

| Change | Who's affected | Migration path |
|---|---|---|
| `gpuOperator.enabled` default flips `true → false` | Anyone whose CD inherits the chart's default | None — the new default removes the (useless) placeholder. Net positive |
| `gpuOperator.enabled=true` now installs real GPU Operator | Anyone explicitly setting `true` for the placeholder semantic | Set `false` to opt out, or accept the new behavior (requires real Linux GPU nodes + mock pools configured in `topology`) |
| `templates/gpu-operator/deployment.yml` deleted | Anyone matching that exact placeholder shape (`replicas: 0`, `ubuntu:22.04`) | Update the check to look for GPU Operator's actual components |
| `nvidia.com/gpu.deploy.*` labels narrowed to mock-pool nodes | Real-Linux-in-fake-pool deployments | Move those nodes into a `backend: mock` pool, or accept that they no longer get GPU-Operator-targeting labels (which they shouldn't have had) |

PR description gets a bullet list of the four rows above.

## Risks

**Profile schema drift with future nvml-mock versions.** Our `templates/profiles/builtin.yaml` is auto-synced from `NVIDIA/k8s-test-infra v0.1.0`. Newer versions may rename or restructure config fields. Mitigation: image pinned to v0.1.0; documented bump procedure (rerun `hack/sync-profiles.sh`, regenerate fixtures, re-run e2e on real hardware).

**DaemonSet shape drift.** We replicate nvml-mock's ~55-line DaemonSet template by hand. Future upstream changes (new env vars, sidecars) require manual port. Mitigation: same as above — pin nvml-mock version, review upstream's `templates/daemonset.yaml` at each image bump, treat the bump as its own PR with re-run e2e.

**GPU Operator chart breaking changes.** Pinning to v26.3.1 buys consistency, but we now own version-bump validation. The exact subchart value key for driver root in v26.3.1 is finalized during implementation — verify by reading the upstream chart's `values.yaml`.

**Day-2 pool reshuffles silently no-op.** Per the Q5 (a) decision, NodeController doesn't watch the topology CM and has no `UpdateFunc`. Status-updater rollout required for backend-flip changes to take effect. Mitigation: documented limitation; revisit when real workflow demands it.

## Out of scope (deferred)

- nvml-mock chart's profile-discovery integration (`integrations.fakeGpuOperator.enabled=true`) — fake-gpu-operator's profile system is the authoritative source, permanently
- Dynamic NodeController updates (`UpdateFunc`, topology-CM watch)
- Multi-cluster / federated mock pools
- Per-pool image overrides for nvml-mock beyond the chain `ResolveImage` already supports
- Bumping the GPU Operator default chart version above v26.3.1

## Success criteria

Spec ships when all of these hold:

1. `helm template` chart-render tests pass for both `gpuOperator.enabled` states
2. Go unit tests pass for all five `_test.go` files in `internal/status-updater/controllers/mock/`
3. `labels_test.go` matrix passes (the four rows in piece C)
4. On a real Linux GPU node provisioned by the user: `helm install` with one mock pool brings up nvml-mock + GPU Operator + a workload requesting `nvidia.com/gpu` runs successfully
5. `gpuOperator.enabled=false` deploy is byte-identical to today's deploy minus the placeholder Deployment (no other regressions)
