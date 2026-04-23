# RUN-38194: Status-Updater as Component Controller

**Status:** Draft  
**Author:** Eliran Wolff  
**Date:** 2026-04-23  
**Branch:** `eliranw/RUN-38194-status-updater-controller`

---

- [Overview](#overview)
- [Goals & Non-Goals](#goals--non-goals)
- [Architecture](#architecture)
  - [High-Level Flow](#high-level-flow)
  - [Component Mapping by Backend](#component-mapping-by-backend)
- [Topology CM Extension](#topology-cm-extension)
  - [Components Section](#components-section)
  - [Image Version Resolution](#image-version-resolution)
- [Reconciliation Loop](#reconciliation-loop)
  - [Desired-State Diffing](#desired-state-diffing)
  - [Managed Resource Labels](#managed-resource-labels)
  - [Node Selectors](#node-selectors)
- [GPU Operator Helm Management](#gpu-operator-helm-management)
  - [Chart Source](#chart-source)
  - [Values Generation](#values-generation)
  - [Lifecycle](#lifecycle)
- [RBAC Requirements](#rbac-requirements)
- [Testing Strategy](#testing-strategy)

---

## Overview

Phase 4 of the FGO redesign evolves the status-updater from a node labeler / topology-CM creator into a **central controller** that programmatically manages all per-pool component deployments. Instead of the Helm chart statically deploying DaemonSets for device-plugin, status-exporter, and KWOK components, the status-updater watches the cluster topology ConfigMap and reconciles the desired component resources dynamically.

This enables:
- **Dynamic pool management** — adding/removing pools doesn't require a Helm upgrade
- **Backend-aware deployments** — `fake` pools get FGO shim components, `mock` pools get GPU Operator
- **Single source of truth** — the topology CM drives everything

## Goals & Non-Goals

### Goals
- Migrate device-plugin, status-exporter, and kwok-dra-plugin DaemonSets from Helm-managed to controller-managed
- Support `fake` backend (FGO device-plugin shim) and `mock` backend (GPU Operator via NVML simulation)
- Manage GPU Operator as a single Helm release scoped to mock pool nodes
- Desired-state reconciliation with create/update/delete semantics
- Clean deletion of resources when pools are removed

### Non-Goals
- `real` backend support (out of scope for this phase)
- Managing non-GPU components (e.g., monitoring, logging)
- Multi-namespace support (all resources in the operator namespace)
- Custom component types beyond the predefined set

## Architecture

### High-Level Flow

```
┌──────────────┐     watches      ┌──────────────────┐
│  Topology CM │ ───────────────> │  ComponentController│
│  (per-cluster)│                  │  (in status-updater)│
└──────────────┘                  └────────┬───────────┘
                                           │
                          ┌────────────────┼────────────────┐
                          │                │                 │
                     fake pools       mock pools        deletion
                          │                │                 │
                          v                v                 v
                   ┌─────────────┐  ┌───────────┐    ┌───────────┐
                   │ DaemonSets  │  │GPU Operator│    │  Remove   │
                   │ per pool:   │  │ single Helm│    │  orphaned │
                   │ - device-   │  │ release    │    │  resources│
                   │   plugin    │  │ (aggregated│    └───────────┘
                   │ - status-   │  │  node      │
                   │   exporter  │  │  selectors)│
                   │ - kwok-dra  │  └───────────┘
                   └─────────────┘
```

### Component Mapping by Backend

| Backend | Components Deployed | Managed By |
|---------|-------------------|------------|
| `fake`  | device-plugin DaemonSet, status-exporter DaemonSet, kwok-dra-plugin DaemonSet | Controller creates K8s resources directly |
| `mock`  | GPU Operator (device-plugin, dcgm-exporter, GFD, etc.) | Controller manages single Helm release |

## Topology CM Extension

The existing cluster topology ConfigMap is extended with an optional `components` section that controls image versions and component-specific configuration.

### Components Section

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-topology
  namespace: fake-gpu-operator
data:
  topology: |
    pools:
      default:
        backend: fake
        nodeCount: 3
        gpuCount: 4
        gpuProfile: A100-SXM4-80GB
      highend:
        backend: mock
        nodeCount: 2
        gpuCount: 8
        gpuProfile: H100-SXM5-80GB

    components:
      # Global defaults — apply to all FGO-managed components
      imageTag: "0.5.0"
      imageRegistry: "gcr.io/run-ai-lab/fake-gpu-operator"

      # Per-component overrides (optional)
      devicePlugin:
        image: "gcr.io/run-ai-lab/fake-gpu-operator/device-plugin:0.6.0-rc1"
      statusExporter: {}      # uses global defaults
      kwokDraPlugin: {}       # uses global defaults

      # GPU Operator config for mock pools
      gpuOperator:
        chartVersion: "24.9.0"
```

### Image Version Resolution

Resolution order (highest priority first):

1. **Per-component `image`** — full image reference (registry/name:tag)
2. **Per-component `imageTag`** + global `imageRegistry` + component default name
3. **Global `imageTag`** + global `imageRegistry` + component default name
4. **Operator's own version** — the tag of the status-updater's own image (fallback)

This means the simplest configuration needs zero `components` config — everything deploys at the same version as the status-updater.

## Reconciliation Loop

### Desired-State Diffing

```
ComponentController.Reconcile():

  1. Read cluster topology CM
  2. For each pool:
     a. Resolve image versions (global → per-component → fallback)
     b. Compute desired resources based on backend:
        - fake → DaemonSet(device-plugin),
                 DaemonSet(status-exporter),
                 DaemonSet(kwok-dra-plugin)
        - mock → (contributes to GPU Operator config)
  3. Aggregate all mock pools → single GPU Operator
     Helm release with combined node selectors
  4. List actual managed resources
     (label: managed-by=fake-gpu-operator)
  5. Diff desired vs actual:
     - Missing in cluster → Create
     - Spec changed       → Update
     - Extra in cluster   → Delete
  6. Apply changes
```

### Managed Resource Labels

All controller-managed resources carry these labels for identification and cleanup:

```yaml
labels:
  app.kubernetes.io/managed-by: fake-gpu-operator
  fake-gpu-operator/component: device-plugin   # device-plugin | status-exporter | kwok-dra-plugin
  fake-gpu-operator/pool: default              # pool name
```

### Node Selectors

Each pool's DaemonSets target only nodes belonging to that pool:

```yaml
nodeSelector:
  run.ai/simulated-gpu-node-pool: "default"
```

## GPU Operator Helm Management

### Chart Source

The GPU Operator Helm chart is pulled from the NVIDIA OCI registry at runtime:

```
oci://nvcr.io/nvidia/gpu-operator
```

**Chart version resolution:**
- Default: configured via Helm values (`gpuOperator.chartVersion` in the FGO chart)
- Override: `components.gpuOperator.chartVersion` in topology CM (per-cluster)

### Values Generation

The controller aggregates all `mock` pool node selectors into a single Helm release. Example with pools `training` and `inference`:

```yaml
operator:
  defaultRuntime: containerd
daemonsets:
  labels:
    app.kubernetes.io/managed-by: fake-gpu-operator
driver:
  enabled: false    # KWOK nodes have no real hardware
toolkit:
  enabled: false
devicePlugin:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: run.ai/simulated-gpu-node-pool
            operator: In
            values: ["training", "inference"]
dcgmExporter:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: run.ai/simulated-gpu-node-pool
            operator: In
            values: ["training", "inference"]
```

### Lifecycle

| Condition | Action |
|-----------|--------|
| First mock pool added | `helm install` GPU Operator |
| Mock pool added/removed | `helm upgrade` with updated node selectors |
| Last mock pool removed | `helm uninstall` GPU Operator |

All Helm operations use `--atomic --wait` to ensure rollback on failure.

## RBAC Requirements

The status-updater ClusterRole must be extended with these permissions:

| Resource | Verbs | Purpose |
|----------|-------|---------|
| ConfigMaps | get, list, watch, create, delete | Topology CMs (existing) |
| DaemonSets | get, list, watch, create, update, delete | Per-pool component DaemonSets |
| Deployments | get, list, watch, create, update, delete | If any component uses Deployment |
| ServiceAccounts | get, list, create, delete | Per-component service accounts |
| ClusterRoles | get, list, create, update, delete | GPU Operator RBAC |
| ClusterRoleBindings | get, list, create, update, delete | GPU Operator RBAC |
| Namespaces | get, list | Namespace reads for Helm |
| Secrets | get, list, create | Helm release storage |

## Testing Strategy

### Unit Tests
- Desired-state computation from topology CM
- Diff logic (create/update/delete decisions)
- Image version resolution chain (per-component → global → fallback)
- Label sanitization on pool names
- Helm values generation for mock pools

### Integration Tests (envtest)
- Create topology CM → verify DaemonSets created with correct labels and node selectors
- Update topology CM (add pool) → verify new DaemonSets appear
- Update topology CM (remove pool) → verify cleanup of orphaned resources
- Update topology CM (change backend) → verify component swap

### E2E Tests (kind cluster)
- Verify device-plugin DaemonSets are created by controller (not Helm)
- Verify pool removal cleans up all associated resources
- Verify GPU Operator install/upgrade/uninstall lifecycle for mock pools
- Verify node labeling still works end-to-end with controller-managed components
