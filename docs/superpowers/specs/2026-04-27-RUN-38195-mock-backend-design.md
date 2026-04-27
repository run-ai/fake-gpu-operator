# Mock Backend Support — Phase 5 Design

**Jira:** [RUN-38195](https://runai.atlassian.net/browse/RUN-38195)
**Date:** 2026-04-27
**Status:** Draft
**Supersedes:** the now-reverted Phase 4 component controller (RUN-38194), whose real customer turned out to be mock pools, not fake pools

## Goal

Support `backend: mock` node pools end-to-end: a real Linux GPU node, scoped to a mock pool, runs upstream NVIDIA GPU stack components against a mocked NVML driver layer (nvml-mock). Both classic device-plugin allocation (GPU Operator) and Dynamic Resource Allocation (`nvidia-dra-driver-gpu`) are supported. Workloads requesting `nvidia.com/gpu` resources or DRA `ResourceClaims` schedule on mock-pool nodes and run as if the cluster had real GPUs.

This spec lands as a single PR. The pieces below are operationally coupled — each is non-functional without the others — so splitting them across PRs would just produce dead intermediate states.

## Architecture

Three layers stack per mock-pool node:

| Layer | What it provides | Owner | Where it lives |
|---|---|---|---|
| **L1: nvml-mock DaemonSet + ConfigMap (per pool)** | Lays down a fake `libnvidia-ml.so`, fake `/dev/nvidia*` device files, profile config at `/var/lib/nvml-mock/driver/` | Status-updater controller (new) | `internal/status-updater/controllers/mock/` |
| **L2a: GPU Operator** | Real device-plugin / GFD / DCGM-exporter, configured to read NVML through L1's mocked driver root | Helm subchart | `deploy/fake-gpu-operator/Chart.yaml`, `values.yaml` |
| **L2b: nvidia-dra-driver-gpu** | Real DRA driver, configured against L1's mocked driver root, exposes GPUs as DRA `ResourceClaim`s | Helm subchart | same |
| **L3: Node labeling** | `nvidia.com/gpu.deploy.*` labels on mock-pool nodes that L2a + L2b components use as nodeSelectors | Status-updater NodeController (existing, narrowed) | `internal/status-updater/handlers/node/labels.go` |

L2a and L2b are independent toggles — a user can enable either, both, or neither (in which case mock-pool support degrades to "nvml-mock runs but nothing consumes it"). Both consume L1's driver root the same way.

End-to-end flow when a user enables mock support:

```
helm install with gpuOperator.enabled=true and/or nvidiaDraDriver.enabled=true
        │
        ├─ Chart renders subcharts conditionally:
        │     L2a: GPU Operator (if gpuOperator.enabled)
        │     L2b: nvidia-dra-driver-gpu (if nvidiaDraDriver.enabled)
        ├─ Status-updater pod starts with MOCK_CONTROLLER_ENABLED=true
        │   (gated on either subchart toggle being on)
        │
        ▼
Status-updater                    Topology CM (with backend: mock pools)
  ├─ NodeController (L3): real Linux nodes labeled with pool key get
  │   nvidia.com/gpu.deploy.* labels — L2a + L2b components target them
  │
  └─ MockController (L1): per mock pool, builds nvml-mock-{pool} DaemonSet
      + nvml-mock-{pool} ConfigMap; reconciles on topology CM changes
        │
        ▼
On a mock-pool node:
  nvml-mock DaemonSet writes /var/lib/nvml-mock/driver/{libnvidia-ml.so, devices, config.yaml}
  L2a (if enabled): real device-plugin/GFD/DCGM-exporter call NVML → see mock GPUs
                    pods requesting nvidia.com/gpu schedule and run
  L2b (if enabled): real DRA driver calls NVML → publishes ResourceSlices
                    pods with ResourceClaims schedule and run
```

## Piece B — Helm subcharts (GPU Operator + DRA driver)

Two upstream charts pulled in as subcharts, each independently toggleable. Both produce real components that consume nvml-mock's driver root for synthetic GPU discovery.

### Chart.yaml dependencies

```yaml
dependencies:
  - name: gpu-operator
    version: "26.3.1"                    # latest as of 2026-04-18
    repository: https://helm.ngc.nvidia.com/nvidia
    condition: gpuOperator.enabled
  - name: nvidia-dra-driver-gpu
    version: "25.12.0"                   # latest as of 2026-02-12
    repository: https://helm.ngc.nvidia.com/nvidia
    condition: nvidiaDraDriver.enabled
```

`gpu-operator` bumps from the previously-stale 24.9.0. No aliases — subchart values keys are the literal hyphenated chart names (`gpu-operator:`, `nvidia-dra-driver-gpu:`), distinct from our parent-chart toggle blocks (`gpuOperator:`, `nvidiaDraDriver:`, camelCase).

### values.yaml structure

```yaml
# Parent-chart-side toggles — our domain
gpuOperator:
  enabled: false                # NOTE: flipped from true → false (see migration)
  chartVersion: "26.3.1"        # informational; actual pin lives in Chart.yaml

nvidiaDraDriver:
  enabled: false                # net-new — no migration concerns
  chartVersion: "25.12.0"       # informational

# Subchart values block — Helm convention, keyed by subchart name.
# Defaults here are the minimum to integrate with nvml-mock. Users add their
# own overrides under these keys; Helm merges them on top.
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

nvidia-dra-driver-gpu:
  nvidiaDriverRoot: /var/lib/nvml-mock/driver  # per nvml-mock README's DRA recipe
  gpuResourcesEnabledOverride: true            # opt-in to DRA-managed GPU resources
  resources:
    computeDomains:
      enabled: false                            # nvml-mock doesn't simulate compute domains
```

User override mechanism: users add keys directly under `gpu-operator:` in their own values file. Helm merges them on top of our defaults. We do **not** introduce a `gpuOperator.values:` passthrough block — that pattern is incompatible with how Helm subcharts consume values (parent values.yaml is not templated).

### Placeholder polyfill (existing behavior preserved)

The existing `templates/gpu-operator/deployment.yml` (placeholder Deployment with `replicas: 0` running `ubuntu:22.04 sleep infinity`) and `crds/nvidia.com_clusterpolicies.yaml` (fake `ClusterPolicy` CRD) exist as **polyfills** for upstream consumers (e.g., RunAI control plane) that detect "is GPU Operator installed?" via these resource shapes. They are not dead code — fake-only deployments depend on them for the GPU-Operator-presence signal.

The semantic for Phase 5 is: the polyfill is rendered *when and only when* the real subchart is not. The gate flips from `{{- if .Values.gpuOperator.enabled -}}` to `{{- if not .Values.gpuOperator.enabled -}}` for both files.

| `gpuOperator.enabled` | Real GPU Operator subchart | Placeholder Deployment + ClusterPolicy CRD |
|---|---|---|
| `false` *(default after Phase 5)* | not installed | rendered (polyfill) |
| `true` | installed via subchart | suppressed (real subchart provides the actual resources) |

This preserves current behavior for fake-only deployments running on chart defaults: today they get the placeholder via `enabled: true`; after Phase 5 they get the placeholder via `enabled: false` (the new default), so no values-side change is required. The flag's semantic shifts from "create the polyfill" to "use the real subchart instead of the polyfill" — same flag, redirected meaning.

**Edge case explicitly accepted:** users who currently set `enabled: false` to suppress the placeholder (e.g., real GPU Operator installed via a separate Helm release out-of-band) will get the polyfill back after Phase 5. Likely zero such users today; if any surface, easy fix is to set `enabled: true` and let our subchart drive.

### Files added

- `deploy/fake-gpu-operator/templates/mock/serviceaccount.yaml` — single `ServiceAccount` named `nvml-mock` in the release namespace, gated on `.Values.gpuOperator.enabled`. Every per-pool DaemonSet built by the controller references it. Lifetime tied to the chart, not to any individual pool.

### Files modified, not removed

- `deploy/fake-gpu-operator/templates/gpu-operator/deployment.yml` — gate flips from `{{- if .Values.gpuOperator.enabled -}}` to `{{- if not .Values.gpuOperator.enabled -}}`. No body change.
- `deploy/fake-gpu-operator/crds/nvidia.com_clusterpolicies.yaml` — same gate flip, if it's currently gated; otherwise add the negative gate at the top.
- `deploy/fake-gpu-operator/templates/gpu-operator/ocp/clusterserviceversion.yaml` is **untouched** — gated independently on `environment.openshift`, serves a different purpose.

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

The Helm template plumbs `MOCK_CONTROLLER_ENABLED` from `or .Values.gpuOperator.enabled .Values.nvidiaDraDriver.enabled`. The controller is needed whenever *either* subchart is consuming nvml-mock's driver root, since both depend on L1.

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
- `gpuOperator.enabled=false`, `nvidiaDraDriver.enabled=false` → polyfill rendered (placeholder Deployment + ClusterPolicy CRD); no subchart resources
- `gpuOperator.enabled=true`, `nvidiaDraDriver.enabled=false` → GPU Operator subchart rendered with our mandatory overrides; polyfill suppressed; nvidia-dra-driver-gpu absent
- `gpuOperator.enabled=false`, `nvidiaDraDriver.enabled=true` → nvidia-dra-driver-gpu subchart rendered with our mandatory overrides (driver root, gpuResourcesEnabledOverride, computeDomains disabled); polyfill **still** rendered (real GPU Operator isn't installed); GPU Operator absent
- `gpuOperator.enabled=true`, `nvidiaDraDriver.enabled=true` → both subcharts rendered; polyfill suppressed
- Both toggles `true` + user overrides under each subchart values key → user values win where they should, mandatory overrides preserved
- `environment.openshift=true` path still produces the OCP `ClusterServiceVersion` independently of either toggle

### End-to-end (real Linux GPU cluster — outside CI)

Two scenarios run on the same provisioned cluster:

**Device-plugin path:**
1. Provision a real Linux node, label with the pool key
2. `helm install` with `gpuOperator.enabled=true`, `nvidiaDraDriver.enabled=false`, and a mock pool defined in `topology`
3. Verify nvml-mock DaemonSet runs, lays down `/var/lib/nvml-mock/driver/`
4. Verify GPU Operator components reach Running, real device-plugin reports `nvidia.com/gpu` allocatable
5. Schedule a pod requesting `nvidia.com/gpu`; verify it lands on the mock node and `nvidia-smi` inside the container talks to nvml-mock

**DRA path (cluster needs Kubernetes 1.32+ with `DynamicResourceAllocation` feature gate):**
1. Same provisioning
2. `helm install` with `gpuOperator.enabled=false`, `nvidiaDraDriver.enabled=true`, and a mock pool defined in `topology`
3. Verify nvml-mock DaemonSet runs (same as above)
4. Verify nvidia-dra-driver-gpu pods reach Running, publishing `ResourceSlice`s with mock GPU devices
5. Schedule a pod with a `ResourceClaim` requesting GPUs; verify it lands on the mock node and `nvidia-smi` inside the container talks to nvml-mock

A combined run (both toggles `true`) is also validated to confirm the two subcharts coexist on the same nodes.

These tests are documented in a runbook for human validation. The PR description records the user's most recent run.

### Out of test scope

- nvml-mock binary itself (upstream's responsibility)
- GPU Operator components (NVIDIA's responsibility)

## Migration impact

| Change | Who's affected | Migration path |
|---|---|---|
| `gpuOperator.enabled` default flips `true → false` | Anyone whose CD inherits the chart's default | **No behavior change.** The new default activates the placeholder polyfill (matches what `enabled: true` did before). Fake-only deployments running on defaults are unaffected. |
| `gpuOperator.enabled=true` now installs real GPU Operator instead of the placeholder | Anyone explicitly setting `true` for the placeholder semantic | Set `false` to keep the placeholder, or accept the new behavior (requires real Linux GPU nodes + mock pools configured in `topology`) |
| `gpuOperator.enabled=false` now activates the placeholder polyfill (was: nothing rendered) | Anyone explicitly setting `false` to suppress both placeholder and real install (e.g., real GPU Operator installed out-of-band) | Set `enabled: true` and let our subchart drive, or open an issue for an explicit polyfill toggle. Likely zero such users. |
| `nvidia.com/gpu.deploy.*` labels narrowed to mock-pool nodes | Real-Linux-in-fake-pool deployments | Move those nodes into a `backend: mock` pool, or accept that they no longer get GPU-Operator-targeting labels (which they shouldn't have had) |

PR description gets a bullet list of the four rows above. Compared to the original draft, the polyfill semantic means *most* users see no change at all on upgrade — only those who explicitly toggled `enabled` away from the default need to act.

## Risks

**Profile schema drift with future nvml-mock versions.** Our `templates/profiles/builtin.yaml` is auto-synced from `NVIDIA/k8s-test-infra v0.1.0`. Newer versions may rename or restructure config fields. Mitigation: image pinned to v0.1.0; documented bump procedure (rerun `hack/sync-profiles.sh`, regenerate fixtures, re-run e2e on real hardware).

**DaemonSet shape drift.** We replicate nvml-mock's ~55-line DaemonSet template by hand. Future upstream changes (new env vars, sidecars) require manual port. Mitigation: same as above — pin nvml-mock version, review upstream's `templates/daemonset.yaml` at each image bump, treat the bump as its own PR with re-run e2e.

**GPU Operator chart breaking changes.** Pinning to v26.3.1 buys consistency, but we now own version-bump validation. The exact subchart value key for driver root in v26.3.1 is finalized during implementation — verify by reading the upstream chart's `values.yaml`.

**`draPlugin` and `nvidiaDraDriver` mutual exclusion.** Our existing `draPlugin` (real-node DRA plugin in `templates/dra-device-plugin/`, defaults `enabled: false`) uses `nvidia.com/gpu.deploy.dra-plugin-gpu: "true"` as its kubeletPlugin nodeSelector — the same label `nvidia-dra-driver-gpu` uses. If a user sets both `draPlugin.enabled: true` and `nvidiaDraDriver.enabled: true`, kubelet plugin registration conflicts on overlapping nodes (only one DRA plugin can register `nvidia.com/gpu` per node). Mitigation: documented incompatibility; consider deprecating `draPlugin` in favor of `nvidiaDraDriver` as a follow-up. Phase 5 does not refactor `draPlugin` — it just adds the new path.

**Day-2 pool reshuffles silently no-op.** Per the Q5 (a) decision, NodeController doesn't watch the topology CM and has no `UpdateFunc`. Status-updater rollout required for backend-flip changes to take effect. Mitigation: documented limitation; revisit when real workflow demands it.

## Out of scope (deferred)

- nvml-mock chart's profile-discovery integration (`integrations.fakeGpuOperator.enabled=true`) — fake-gpu-operator's profile system is the authoritative source, permanently
- Dynamic NodeController updates (`UpdateFunc`, topology-CM watch)
- Multi-cluster / federated mock pools
- Per-pool image overrides for nvml-mock beyond the chain `ResolveImage` already supports
- Bumping the GPU Operator default chart version above v26.3.1, or nvidia-dra-driver-gpu above v25.12.0
- Refactoring or deprecating the existing `draPlugin` to avoid the conflict with `nvidiaDraDriver` (a follow-up cleanup)

## Success criteria

Spec ships when all of these hold:

1. `helm template` chart-render tests pass for the matrix of `gpuOperator.enabled` × `nvidiaDraDriver.enabled` states (4 combinations)
2. Go unit tests pass for all five `_test.go` files in `internal/status-updater/controllers/mock/`
3. `labels_test.go` matrix passes (the four rows in piece C)
4. On a real Linux GPU node provisioned by the user, both e2e scenarios pass:
   - `helm install` with `gpuOperator.enabled=true` + a mock pool → `nvidia.com/gpu` workload runs
   - `helm install` with `nvidiaDraDriver.enabled=true` + a mock pool (Kubernetes 1.32+ with DRA feature gate) → DRA `ResourceClaim` workload runs
5. `gpuOperator.enabled=false` + `nvidiaDraDriver.enabled=false` deploy is byte-identical to today's `gpuOperator.enabled=true` deploy (polyfill activates on the new default; no values change required for fake-only deployments)
