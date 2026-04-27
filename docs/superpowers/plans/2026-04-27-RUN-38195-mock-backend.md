# Mock Backend Support Implementation Plan (RUN-38195)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship Phase 5 mock-backend support — `backend: mock` node pools serve real NVIDIA GPU stack components (GPU Operator and/or DRA driver) against a mocked NVML driver layer (nvml-mock), with per-pool resources reconciled by a new status-updater controller.

**Architecture:** Three pieces ship as one PR. (A) A new `internal/status-updater/controllers/mock/` package builds per-pool nvml-mock DaemonSets + ConfigMaps directly (no Helm SDK). (B) Two upstream Helm subcharts (`gpu-operator` + `nvidia-dra-driver-gpu`) are added with mandatory overrides that point both at `/var/lib/nvml-mock/driver`; the existing GPU-Operator placeholder Deployment + ClusterPolicy CRD become a polyfill (gate inverts). (C) `internal/status-updater/handlers/node/labels.go` narrows its predicate so `nvidia.com/gpu.deploy.*` labels apply only to nodes whose pool has `Gpu.Backend == "mock"`.

**Tech Stack:** Go 1.x, `client-go` (informers + fake clientset), `testify` (unit tests), Helm 3 (subchart dependencies + chart-template tests), upstream `nvml-mock v0.1.0`, `gpu-operator 26.3.1`, `nvidia-dra-driver-gpu 25.12.0`.

**Spec:** [`docs/superpowers/specs/2026-04-27-RUN-38195-mock-backend-design.md`](../specs/2026-04-27-RUN-38195-mock-backend-design.md) (committed `e10f787`).

---

## File map

```
deploy/fake-gpu-operator/
├── Chart.yaml                                                    # MODIFY: add 2 subchart deps
├── values.yaml                                                   # MODIFY: toggles + subchart values
├── crds/nvidia.com_clusterpolicies.yaml                          # MODIFY: invert gate
└── templates/
    ├── gpu-operator/deployment.yml                               # MODIFY: invert gate (polyfill)
    ├── mock/serviceaccount.yaml                                  # CREATE
    └── status-updater/
        ├── deployment.yaml                                       # MODIFY: plumb MOCK_CONTROLLER_ENABLED env
        └── clusterrole.yaml                                      # MODIFY: daemonsets RBAC
test/helm/
└── mock_backend_test.sh                                          # CREATE: helm template render matrix

internal/
├── common/
│   ├── constants/constants.go                                    # MODIFY: add EnvMockControllerEnabled
│   └── topology/types.go                                         # MODIFY: add NvmlMock to ComponentsConfig
└── status-updater/
    ├── app.go                                                    # MODIFY: wire MockController behind env flag
    ├── controllers/mock/                                         # CREATE: new package
    │   ├── controller.go                                         #   informer on topology CM
    │   ├── controller_test.go
    │   ├── reconciler.go                                         #   readConfig → desired → diff → apply
    │   ├── reconciler_test.go
    │   ├── desired_state.go                                      #   pure: ClusterConfig → []runtime.Object
    │   ├── desired_state_test.go
    │   ├── resources.go                                          #   DaemonSet + ConfigMap builders
    │   ├── resources_test.go
    │   ├── diff.go                                               #   DaemonSetDiff + ConfigMapDiff
    │   ├── diff_test.go
    │   ├── profile.go                                            #   profile lookup + override merge
    │   └── profile_test.go
    └── handlers/node/
        ├── labels.go                                             # MODIFY: narrow predicate to mock pools
        └── labels_test.go                                        # CREATE: 4-row behavior matrix
```

**Test framework:** Use `testify/assert` + `testify/require` + `client-go/kubernetes/fake` (matches `internal/common/profile/profile_test.go` style). Do **not** use Ginkgo/Gomega for the new package — keep it consistent with the most recent test additions in this codebase.

---

## Task 1: Branch setup

**Files:** *(none — git operations only)*

The spec is currently on `eliranw/RUN-38194-status-updater-controller`. Phase 5 should land on a fresh branch off `main`.

- [ ] **Step 1: Confirm current branch and spec commit**

Run:
```bash
git branch --show-current
git log -1 --oneline -- docs/superpowers/specs/2026-04-27-RUN-38195-mock-backend-design.md
```

Expected: branch is `eliranw/RUN-38194-status-updater-controller`; spec commit is `e10f787`.

- [ ] **Step 2: Create the Phase 5 branch off main, cherry-pick the spec**

Run:
```bash
git fetch origin main
git checkout -b eliranw/RUN-38195-mock-backend origin/main
git cherry-pick e10f787 600982b 9064a5a 2>&1 || true
git log --oneline main..HEAD
```

Expected: branch contains the spec commit, the spec-extension commit, and the design-doc-removal commit, on top of `main`. (The plan commit will be added in Step 3.)

- [ ] **Step 3: Commit this plan onto the new branch**

Run:
```bash
git add docs/superpowers/plans/2026-04-27-RUN-38195-mock-backend.md
git commit -m "docs: add Phase 5 mock backend implementation plan (RUN-38195)"
```

---

## Task 2: Add managed-resource label constants + `ComponentNvmlMock` ✓ DONE

**Files:**
- Modify: `internal/common/constants/constants.go`

We need stable label key/value constants the mock controller can stamp onto per-pool DaemonSets and ConfigMaps so listings can be filtered by them. The plan's original anchor (`LabelManagedByValue`) didn't actually exist on this branch — those constants were Phase-4-only — so they're added in this task.

**Design note:** the controller is *not* gated by a `MOCK_CONTROLLER_ENABLED` env var. It runs unconditionally inside status-updater — when no `backend: mock` pools exist in the topology CM it produces an empty desired set and does no work. Tasks 10 and 20 are also affected (see notes there).

- [x] **Step 1: Add the four managed-resource label constants + the component identifier**

Added to `internal/common/constants/constants.go`:

```go
// Managed-resource labels — applied to all per-pool resources the
// status-updater controller manages, so listings can filter by them.
LabelManagedBy      = "app.kubernetes.io/managed-by"
LabelManagedByValue = "fake-gpu-operator"
LabelComponent      = "fake-gpu-operator/component"
LabelPool           = "fake-gpu-operator/pool"

// Component identifier values for LabelComponent.
ComponentNvmlMock = "nvml-mock"
```

- [x] **Step 2: Build to verify** — `go build ./...` clean

- [x] **Step 3: Commit** — landed as two commits: `7b07842` (ComponentNvmlMock) + `8aad018` (the four Label* constants alongside)

---

## Task 3: ~~Add `NvmlMock` field to `ComponentsConfig`~~ — **DELETED**

`ComponentsConfig` and the `ResolveImage` chain were Phase-4-only additions that don't exist on this branch. Adding them back just to flow one image string through is overkill — every mock pool runs the same upstream nvml-mock binary, so per-pool image overrides have no real use case.

**Replacement design:** the chart plumbs a single `NVML_MOCK_IMAGE` env var (e.g. `ghcr.io/nvidia/nvml-mock:v0.1.0`) on the status-updater Deployment, sourced from `nvmlMock.image.repository` + `nvmlMock.image.tag` in values.yaml. The mock controller reads this env once at startup and stamps it onto every per-pool DaemonSet it builds. See Tasks 7, 10, 15, 17, 20 for the affected places.

Skip directly to Task 4.

---

---

## Task 4: ~~Piece C — labels.go behavior matrix test~~ — **DELETED**

Piece C is unnecessary. NVIDIA's canonical config for nvml-mock + GPU Operator integration (`tests/e2e/gpu-operator-values.yaml` in `NVIDIA/k8s-test-infra`) leaves NFD enabled. NFD chains `nvidia.com/gpu.present=true` (applied by nvml-mock) into the `nvidia.com/gpu.deploy.*` labels GPU Operator's reconciler uses for component placement. Manual labeling via NodeController is redundant.

The `BackendFake`/`BackendMock` constants added during the original Task 4 implementation are kept (mock controller still needs them); only the labels_test.go was reverted (`553cf2d`). Skip directly to Task 6.

(See updated Task 7 subchart values block — it now follows NVIDIA's canonical config with NFD enabled and CDI mode.)

---

## Task 5: ~~Piece C — make labels_test.go pass~~ — **DELETED**

See Task 4 deletion note. labels.go is unchanged in this PR.

---

## Task 6: Piece B — Chart.yaml subchart dependencies

**Files:**
- Modify: `deploy/fake-gpu-operator/Chart.yaml`

- [ ] **Step 1: Append `dependencies` block**

Open `deploy/fake-gpu-operator/Chart.yaml`. Append at the end of the file:

```yaml
dependencies:
  - name: gpu-operator
    version: "26.3.1"
    repository: https://helm.ngc.nvidia.com/nvidia
    condition: gpuOperator.enabled
  - name: nvidia-dra-driver-gpu
    version: "25.12.0"
    repository: https://helm.ngc.nvidia.com/nvidia
    condition: nvidiaDraDriver.enabled
```

- [ ] **Step 2: Run helm dependency update to verify the deps resolve**

Run: `helm dependency update deploy/fake-gpu-operator`
Expected: both charts download into `deploy/fake-gpu-operator/charts/`. Output mentions both `gpu-operator-26.3.1.tgz` and `nvidia-dra-driver-gpu-25.12.0.tgz`.

- [ ] **Step 3: Add `charts/` to `.helmignore` if not already there**

Check `deploy/fake-gpu-operator/.helmignore` (or repo `.gitignore`) — chart `tgz`s should not be committed:

```bash
grep -E "^charts/?$" deploy/fake-gpu-operator/.helmignore .gitignore 2>&1 || echo "MISSING"
```

If MISSING in both, add `charts/` to `deploy/fake-gpu-operator/.helmignore`.

- [ ] **Step 4: Commit Chart.yaml + .helmignore changes**

```bash
git add deploy/fake-gpu-operator/Chart.yaml deploy/fake-gpu-operator/.helmignore
git commit -m "feat(chart): add gpu-operator + nvidia-dra-driver-gpu subchart deps (RUN-38195)"
```

---

## Task 7: Piece B — values.yaml toggles + subchart values blocks

**Files:**
- Modify: `deploy/fake-gpu-operator/values.yaml`

- [ ] **Step 1: Update the `gpuOperator` block — flip default, add `nvidiaDraDriver`**

Find the existing `gpuOperator:` block. Replace it with:

```yaml
gpuOperator:
  # When true, installs the upstream NVIDIA GPU Operator chart as a subchart.
  # When false (default), renders the placeholder Deployment + ClusterPolicy CRD
  # polyfill so upstream consumers (RunAI control plane, etc.) still detect
  # GPU Operator's presence on fake-only deployments.
  enabled: false
  # Informational — the actual subchart pin lives in Chart.yaml.
  chartVersion: "26.3.1"

# When true, installs nvidia-dra-driver-gpu as a subchart for DRA-based GPU
# allocation on mock-pool nodes. Independent of gpuOperator.enabled.
nvidiaDraDriver:
  enabled: false
  # Informational — actual pin lives in Chart.yaml.
  chartVersion: "25.12.0"
```

- [ ] **Step 2: Add `nvmlMock` image config block**

Add a new top-level block (place near the other component image blocks like `devicePlugin`):

```yaml
# Image config for the per-pool nvml-mock DaemonSet built by the mock controller.
# Only consumed when gpuOperator.enabled or nvidiaDraDriver.enabled is true.
nvmlMock:
  image:
    repository: ghcr.io/nvidia/nvml-mock
    tag: "v0.1.0"
    pullPolicy: IfNotPresent
```

- [ ] **Step 3: Add subchart values blocks**

Add two top-level blocks at the end of `values.yaml` (each keyed by hyphenated subchart name per Helm convention).

The `gpu-operator:` block adopts NVIDIA's canonical config from
`tests/e2e/gpu-operator-values.yaml` in `NVIDIA/k8s-test-infra` — the recipe NVIDIA themselves use for nvml-mock + GPU Operator integration. Users can override anything by adding keys under `gpu-operator:` in their own values file; Helm merges on top.

```yaml
# Subchart values block for upstream gpu-operator chart.
# Adopts NVIDIA's canonical nvml-mock + GPU Operator config:
#   https://github.com/NVIDIA/k8s-test-infra/blob/main/tests/e2e/gpu-operator-values.yaml
gpu-operator:
  # nvml-mock provides the driver root and mock libs directly; no real
  # kernel module or container toolkit needed.
  driver:
    enabled: false
  toolkit:
    enabled: false

  # DCGM and dependents — mock NVML doesn't support DCGM (requires full
  # driver stack), so these are off.
  dcgm:
    enabled: false
  dcgmExporter:
    enabled: false
  nodeStatusExporter:
    enabled: false

  # MIG — mock NVML returns NOT_FOUND for MIG queries; strategy must be
  # "none" or the device-plugin treats the responses as fatal.
  mig:
    strategy: none
  migManager:
    enabled: false

  # CDI mode — toolkit reads /var/run/cdi/nvidia.yaml emitted by nvml-mock.
  cdi:
    enabled: true
    default: true

  # NFD STAYS ENABLED. NFD detects nvml-mock's nvidia.com/gpu.present=true
  # label and chains the nvidia.com/gpu.deploy.* labels GPU Operator's
  # reconciler uses for component placement. We do NOT manually apply
  # those labels via NodeController.

  # Device plugin discovers GPUs via mock NVML.
  devicePlugin:
    enabled: true
    config:
      name: ""
    env:
      - name: NVIDIA_DRIVER_ROOT
        value: /var/lib/nvml-mock/driver

  # GFD reads GPU attributes via mock NVML and labels the node.
  gfd:
    enabled: true
    env:
      - name: NVIDIA_DRIVER_ROOT
        value: /var/lib/nvml-mock/driver

  # Validator checks the GPU stack is functional, with mock-aware env vars:
  #   driver-validation: hostPath /run/nvidia/driver → our mock (via symlink)
  #   toolkit-validation: CDI injection makes nvidia-smi available
  #   cuda-validation: WITH_WORKLOAD=false skips kernel launch (no-op on mock)
  #   plugin-validation: checks nvidia.com/gpu in node allocatable
  validator:
    driver:
      env:
        - name: DRIVER_INSTALL_DIR
          value: /run/nvidia/driver
        - name: LD_LIBRARY_PATH
          value: /run/nvidia/driver/usr/lib64
        - name: DISABLE_DEV_CHAR_SYMLINK_CREATION
          value: "true"
    toolkit:
      env:
        - name: NVIDIA_VISIBLE_DEVICES
          value: all
    cuda:
      env:
        - name: WITH_WORKLOAD
          value: "false"
    plugin:
      env:
        - name: LD_LIBRARY_PATH
          value: /run/nvidia/driver/usr/lib64

# Subchart values block for nvidia-dra-driver-gpu.
# Defaults follow the nvml-mock README's DRA recipe.
nvidia-dra-driver-gpu:
  nvidiaDriverRoot: /var/lib/nvml-mock/driver
  gpuResourcesEnabledOverride: true
  resources:
    computeDomains:
      enabled: false
```

- [ ] **Step 4: Render the chart with default values to confirm parsing**

Run: `helm template fgo deploy/fake-gpu-operator > /dev/null`
Expected: clean render, no errors.

- [ ] **Step 5: Commit**

```bash
git add deploy/fake-gpu-operator/values.yaml
git commit -m "feat(chart): add gpuOperator/nvidiaDraDriver toggles + subchart values (RUN-38195)"
```

---

## Task 8: Piece B — nvml-mock ServiceAccount template

**Files:**
- Create: `deploy/fake-gpu-operator/templates/mock/serviceaccount.yaml`

A single chart-templated `ServiceAccount` referenced by every per-pool DaemonSet the controller builds.

- [ ] **Step 1: Create the template**

Create `deploy/fake-gpu-operator/templates/mock/serviceaccount.yaml`:

```yaml
{{- if or .Values.gpuOperator.enabled .Values.nvidiaDraDriver.enabled -}}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: nvml-mock
  namespace: {{ .Release.Namespace }}
  labels:
    app.kubernetes.io/managed-by: fake-gpu-operator
    fake-gpu-operator/component: nvml-mock
{{- end -}}
```

- [ ] **Step 2: Render with each toggle to confirm it appears**

Run:
```bash
helm template fgo deploy/fake-gpu-operator --set gpuOperator.enabled=true | grep -A 2 "kind: ServiceAccount" | grep "nvml-mock"
helm template fgo deploy/fake-gpu-operator --set nvidiaDraDriver.enabled=true | grep -A 2 "kind: ServiceAccount" | grep "nvml-mock"
helm template fgo deploy/fake-gpu-operator | grep -c "name: nvml-mock"
```

Expected: first two commands produce one match each (the SA appears); third command outputs `0` (default values render no nvml-mock SA).

- [ ] **Step 3: Commit**

```bash
git add deploy/fake-gpu-operator/templates/mock/serviceaccount.yaml
git commit -m "feat(chart): add nvml-mock ServiceAccount gated on subchart toggles (RUN-38195)"
```

---

## Task 9: Piece B — invert placeholder polyfill gates

**Files:**
- Modify: `deploy/fake-gpu-operator/templates/gpu-operator/deployment.yml`
- Modify: `deploy/fake-gpu-operator/crds/nvidia.com_clusterpolicies.yaml`

- [ ] **Step 1: Flip the placeholder Deployment gate**

Open `deploy/fake-gpu-operator/templates/gpu-operator/deployment.yml`. Change line 1 from:

```yaml
{{- if .Values.gpuOperator.enabled -}}
```

to:

```yaml
{{- if not .Values.gpuOperator.enabled -}}
```

Leave the rest of the file unchanged.

- [ ] **Step 2: Inspect ClusterPolicy CRD's current gating**

Run: `head -3 deploy/fake-gpu-operator/crds/nvidia.com_clusterpolicies.yaml`

If the file starts with `{{- if .Values.gpuOperator.enabled -}}`, flip it to `{{- if not .Values.gpuOperator.enabled -}}` (matching `{{- end -}}` should already exist at the bottom).

If the file is **not** templated (starts with `apiVersion:`), prepend a gate line and append a closing line:
- Add at the very top: `{{- if not .Values.gpuOperator.enabled -}}`
- Add at the very bottom: `{{- end -}}`

- [ ] **Step 3: Verify polyfill renders by default and disappears when enabled**

Run:
```bash
helm template fgo deploy/fake-gpu-operator | grep -c "name: gpu-operator"
helm template fgo deploy/fake-gpu-operator --set gpuOperator.enabled=true | grep -c "kind: ClusterPolicy"
```

Expected: first command outputs at least `1` (the placeholder Deployment renders by default); second outputs `0` (real subchart suppresses the fake CRD).

- [ ] **Step 4: Commit**

```bash
git add deploy/fake-gpu-operator/templates/gpu-operator/deployment.yml \
        deploy/fake-gpu-operator/crds/nvidia.com_clusterpolicies.yaml
git commit -m "feat(chart): invert polyfill gate so it activates when subchart is off (RUN-38195)"
```

---

## Task 10: Piece B — plumb `NVML_MOCK_IMAGE` env

**Files:**
- Modify: `deploy/fake-gpu-operator/templates/status-updater/deployment.yaml`

The mock controller reads the nvml-mock image as a single env var. The chart constructs `repository:tag` from `nvmlMock.image.*` (set in Task 7) and plumbs it as `NVML_MOCK_IMAGE`.

- [ ] **Step 1: Find the status-updater container's env block**

Open `deploy/fake-gpu-operator/templates/status-updater/deployment.yaml`. Locate the `env:` list under the status-updater container.

- [ ] **Step 2: Append the new env entry**

Add (preserve indentation matching adjacent entries):

```yaml
            - name: NVML_MOCK_IMAGE
              value: "{{ .Values.nvmlMock.image.repository }}:{{ .Values.nvmlMock.image.tag }}"
```

- [ ] **Step 3: Verify rendering**

Run:
```bash
helm template fgo deploy/fake-gpu-operator | grep -A 1 "NVML_MOCK_IMAGE"
```

Expected output:
```
- name: NVML_MOCK_IMAGE
  value: "ghcr.io/nvidia/nvml-mock:v0.1.0"
```

- [ ] **Step 4: Commit**

```bash
git add deploy/fake-gpu-operator/templates/status-updater/deployment.yaml
git commit -m "feat(chart): plumb NVML_MOCK_IMAGE env from nvmlMock.image values (RUN-38195)"
```

---

## Task 11: Piece B — ClusterRole RBAC for daemonsets

**Files:**
- Modify: `deploy/fake-gpu-operator/templates/status-updater/clusterrole.yaml`

The mock controller creates/updates/deletes per-pool DaemonSets. It needs `apps/daemonsets` verbs the existing role doesn't have.

- [ ] **Step 1: Inspect current ClusterRole**

Run: `cat deploy/fake-gpu-operator/templates/status-updater/clusterrole.yaml`

Identify the `rules:` list. Note whether `apps/daemonsets` is already present (it isn't on `main`).

- [ ] **Step 2: Append the daemonsets rule**

Add a new rule entry:

```yaml
- apiGroups:
    - apps
  resources:
    - daemonsets
  verbs:
    - get
    - list
    - watch
    - create
    - update
    - patch
    - delete
```

Place it adjacent to other `apps` rules if any exist, otherwise at the end of the `rules:` block.

- [ ] **Step 3: Verify rendering**

Run:
```bash
helm template fgo deploy/fake-gpu-operator | grep -B 1 -A 8 "resources:" | grep -A 6 "daemonsets"
```

Expected: the daemonsets resource block appears with all seven verbs.

- [ ] **Step 4: Commit**

```bash
git add deploy/fake-gpu-operator/templates/status-updater/clusterrole.yaml
git commit -m "feat(chart): grant status-updater apps/daemonsets RBAC (RUN-38195)"
```

---

## Task 12: Piece B — chart-render matrix tests

**Files:**
- Create: `test/helm/mock_backend_test.sh`

Six render-matrix assertions covering the polyfill / toggle / subchart presence cases from the spec.

- [ ] **Step 1: Create the test script**

Create `test/helm/mock_backend_test.sh`:

```bash
#!/usr/bin/env bash
# Helm chart-render assertions for Phase 5 mock-backend support (RUN-38195).
# Runs `helm template` with various toggle combinations and greps the rendered
# manifest for expected/expected-absent resource shapes.
set -euo pipefail

CHART="${CHART:-deploy/fake-gpu-operator}"
PASS=0
FAIL=0

assert_contains() {
    local label="$1"
    local pattern="$2"
    local rendered="$3"
    if echo "$rendered" | grep -qE "$pattern"; then
        echo "PASS: $label"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $label  (pattern not found: $pattern)"
        FAIL=$((FAIL + 1))
    fi
}

assert_absent() {
    local label="$1"
    local pattern="$2"
    local rendered="$3"
    if echo "$rendered" | grep -qE "$pattern"; then
        echo "FAIL: $label  (unexpected match: $pattern)"
        FAIL=$((FAIL + 1))
    else
        echo "PASS: $label"
        PASS=$((PASS + 1))
    fi
}

render() {
    helm template fgo "$CHART" "$@"
}

echo "=== Case 1: defaults (both toggles false) ==="
R=$(render)
assert_contains "polyfill placeholder Deployment renders" "name: gpu-operator" "$R"
assert_absent   "GPU Operator subchart absent"            "app.kubernetes.io/name: gpu-operator-node-feature-discovery" "$R"
assert_absent   "DRA driver subchart absent"              "name: nvidia-dra-driver-gpu" "$R"
assert_absent   "nvml-mock SA absent"                     "name: nvml-mock" "$R"

echo "=== Case 2: gpuOperator.enabled=true, nvidiaDraDriver.enabled=false ==="
R=$(render --set gpuOperator.enabled=true)
assert_absent   "polyfill suppressed when subchart on"    "image: ubuntu:22.04" "$R"
assert_contains "GPU Operator subchart present"           "operator.role.kubernetes.io/operator|gpu-operator-master|nfd-master" "$R"
assert_absent   "DRA driver subchart absent"              "name: nvidia-dra-driver-gpu-kubeletplugin" "$R"
assert_contains "nvml-mock SA present"                    "name: nvml-mock" "$R"
assert_contains "MOCK_CONTROLLER_ENABLED=true"            "MOCK_CONTROLLER_ENABLED" "$R"

echo "=== Case 3: gpuOperator.enabled=false, nvidiaDraDriver.enabled=true ==="
R=$(render --set nvidiaDraDriver.enabled=true)
assert_contains "polyfill still rendered (real GPU Op absent)" "image: ubuntu:22.04" "$R"
assert_contains "DRA driver subchart present"            "kubeletplugin|dra-driver-gpu" "$R"
assert_contains "nvml-mock SA present"                   "name: nvml-mock" "$R"

echo "=== Case 4: both toggles true ==="
R=$(render --set gpuOperator.enabled=true --set nvidiaDraDriver.enabled=true)
assert_absent   "polyfill suppressed"                    "image: ubuntu:22.04" "$R"
assert_contains "GPU Operator subchart present"          "gpu-operator|nfd-master" "$R"
assert_contains "DRA driver subchart present"            "kubeletplugin|dra-driver-gpu" "$R"

echo "=== Case 5: OCP CSV is independent of toggles ==="
R_DEFAULT=$(render --set environment.openshift=true)
R_BOTH=$(render --set environment.openshift=true --set gpuOperator.enabled=true)
assert_contains "OCP CSV with default toggles"           "kind: ClusterServiceVersion" "$R_DEFAULT"
assert_contains "OCP CSV with subchart on"               "kind: ClusterServiceVersion" "$R_BOTH"

echo
echo "=== Summary: $PASS passed, $FAIL failed ==="
[ "$FAIL" -eq 0 ]
```

- [ ] **Step 2: Make it executable**

Run: `chmod +x test/helm/mock_backend_test.sh`

- [ ] **Step 3: First run — expected failures will reveal subchart-rendered resource shapes**

Run: `helm dependency update deploy/fake-gpu-operator && test/helm/mock_backend_test.sh`

Expected outcome of *first* run: most cases PASS, but the patterns matching subchart resources (`gpu-operator-master|nfd-master`, `kubeletplugin|dra-driver-gpu`) may need adjustment based on what v26.3.1 / v25.12.0 actually render. **Adjust the patterns if needed** by inspecting the real rendered output:
```bash
helm template fgo deploy/fake-gpu-operator --set gpuOperator.enabled=true | grep -E "kind: (Deployment|DaemonSet)" | head -10
```

- [ ] **Step 4: Iterate until all 5 cases pass**

Run: `test/helm/mock_backend_test.sh`
Expected: `=== Summary: <N> passed, 0 failed ===` and exit 0.

- [ ] **Step 5: Commit**

```bash
git add test/helm/mock_backend_test.sh
git commit -m "test(chart): helm template render matrix for mock backend toggles (RUN-38195)"
```

---

## Task 13: Piece A — scaffold the mock package

**Files:**
- Create: `internal/status-updater/controllers/mock/doc.go`

A trivial first commit that establishes the package and lets later tests import it.

- [ ] **Step 1: Create package doc file**

Create `internal/status-updater/controllers/mock/doc.go`:

```go
// Package mock implements the status-updater MockController. It watches
// the cluster topology ConfigMap and reconciles per-pool nvml-mock
// DaemonSets and ConfigMaps for pools whose backend is "mock".
//
// The controller is gated by the MOCK_CONTROLLER_ENABLED env var, which
// the Helm chart sets to (gpuOperator.enabled OR nvidiaDraDriver.enabled).
//
// This package does NOT use the Helm SDK at runtime — per-pool resources
// are built directly as Kubernetes objects, mirroring the shape of
// upstream nvml-mock's templates/daemonset.yaml.
//
// Spec: docs/superpowers/specs/2026-04-27-RUN-38195-mock-backend-design.md
package mock
```

- [ ] **Step 2: Build to confirm package compiles**

Run: `go build ./internal/status-updater/controllers/mock/...`
Expected: clean build.

- [ ] **Step 3: Commit**

```bash
git add internal/status-updater/controllers/mock/doc.go
git commit -m "feat: scaffold mock controller package (RUN-38195)"
```

---

## Task 14: Piece A — `profile.go` (lookup + override merge)

**Files:**
- Create: `internal/status-updater/controllers/mock/profile.go`
- Create: `internal/status-updater/controllers/mock/profile_test.go`

Adapter on top of `internal/common/profile`: load profile CM by name, deep-merge overrides, return serialized YAML for the per-pool ConfigMap's `config.yaml` key.

- [ ] **Step 1: Write the failing tests**

Create `internal/status-updater/controllers/mock/profile_test.go`:

```go
package mock

import (
	"testing"

	commonprofile "github.com/run-ai/fake-gpu-operator/internal/common/profile"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/yaml"
)

const a100ProfileYAML = `
version: "1.0"
system:
  driver_version: "550.163.01"
  cuda_version: "12.4"
device_defaults:
  name: "NVIDIA A100-SXM4-40GB"
  architecture: "ampere"
devices:
  - index: 0
    uuid: "GPU-00000000-0000-0000-0000-000000000000"
`

func newProfileCM(name, ns, data string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    map[string]string{commonprofile.LabelGpuProfile: "true"},
		},
		Data: map[string]string{commonprofile.CmProfileKey: data},
	}
}

func TestRenderConfig_NoOverrides(t *testing.T) {
	cm := newProfileCM("gpu-profile-a100", "ns", a100ProfileYAML)
	kube := fake.NewSimpleClientset(cm)

	out, err := RenderConfig(kube, "ns", topology.GpuConfig{Profile: "a100"})
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, yaml.Unmarshal(out, &got))
	system := got["system"].(map[string]interface{})
	assert.Equal(t, "550.163.01", system["driver_version"])
}

func TestRenderConfig_ScalarOverride(t *testing.T) {
	cm := newProfileCM("gpu-profile-a100", "ns", a100ProfileYAML)
	kube := fake.NewSimpleClientset(cm)

	out, err := RenderConfig(kube, "ns", topology.GpuConfig{
		Profile: "a100",
		Overrides: map[string]interface{}{
			"system": map[string]interface{}{
				"driver_version": "999.99.99",
			},
		},
	})
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, yaml.Unmarshal(out, &got))
	system := got["system"].(map[string]interface{})
	assert.Equal(t, "999.99.99", system["driver_version"])
	assert.Equal(t, "12.4", system["cuda_version"], "non-overridden keys preserved")
}

func TestRenderConfig_NestedOverride(t *testing.T) {
	cm := newProfileCM("gpu-profile-a100", "ns", a100ProfileYAML)
	kube := fake.NewSimpleClientset(cm)

	out, err := RenderConfig(kube, "ns", topology.GpuConfig{
		Profile: "a100",
		Overrides: map[string]interface{}{
			"device_defaults": map[string]interface{}{
				"architecture": "custom-arch",
			},
		},
	})
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, yaml.Unmarshal(out, &got))
	dd := got["device_defaults"].(map[string]interface{})
	assert.Equal(t, "custom-arch", dd["architecture"])
	assert.Equal(t, "NVIDIA A100-SXM4-40GB", dd["name"], "non-overridden nested keys preserved")
}

func TestRenderConfig_AddNewKey(t *testing.T) {
	cm := newProfileCM("gpu-profile-a100", "ns", a100ProfileYAML)
	kube := fake.NewSimpleClientset(cm)

	out, err := RenderConfig(kube, "ns", topology.GpuConfig{
		Profile: "a100",
		Overrides: map[string]interface{}{
			"new_section": map[string]interface{}{
				"flag": true,
			},
		},
	})
	require.NoError(t, err)

	var got map[string]interface{}
	require.NoError(t, yaml.Unmarshal(out, &got))
	newSec := got["new_section"].(map[string]interface{})
	assert.Equal(t, true, newSec["flag"])
}

func TestRenderConfig_MissingProfileCM(t *testing.T) {
	kube := fake.NewSimpleClientset()
	_, err := RenderConfig(kube, "ns", topology.GpuConfig{Profile: "h100"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "h100")
}

func TestRenderConfig_MalformedProfileYAML(t *testing.T) {
	cm := newProfileCM("gpu-profile-broken", "ns", "this: is: not: yaml: nested: bad")
	kube := fake.NewSimpleClientset(cm)
	_, err := RenderConfig(kube, "ns", topology.GpuConfig{Profile: "broken"})
	require.Error(t, err)
}

func TestRenderConfig_EmptyProfile(t *testing.T) {
	out, err := RenderConfig(fake.NewSimpleClientset(), "ns", topology.GpuConfig{})
	assert.Error(t, err, "empty profile name must error")
	assert.Nil(t, out)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/status-updater/controllers/mock/... -run RenderConfig -v`
Expected: all fail with `undefined: RenderConfig`.

- [ ] **Step 3: Implement `profile.go`**

Create `internal/status-updater/controllers/mock/profile.go`:

```go
package mock

import (
	"fmt"

	commonprofile "github.com/run-ai/fake-gpu-operator/internal/common/profile"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

// RenderConfig produces the serialized YAML body for a per-pool nvml-mock
// ConfigMap's `config.yaml` key. It loads the profile ConfigMap referenced
// by gpu.Profile, deep-merges gpu.Overrides on top using the existing
// common/profile helpers, and serializes the result.
//
// Returns an error if the profile name is empty, the profile ConfigMap is
// missing, or the profile YAML is malformed.
func RenderConfig(kube kubernetes.Interface, namespace string, gpu topology.GpuConfig) ([]byte, error) {
	if gpu.Profile == "" {
		return nil, fmt.Errorf("mock pool requires a non-empty profile")
	}

	base, err := commonprofile.Load(kube, namespace, gpu.Profile)
	if err != nil {
		return nil, fmt.Errorf("loading profile %q: %w", gpu.Profile, err)
	}

	merged := commonprofile.Merge(base, gpu.Overrides)

	out, err := yaml.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("serializing merged profile: %w", err)
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/status-updater/controllers/mock/... -run RenderConfig -v`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/status-updater/controllers/mock/profile.go \
        internal/status-updater/controllers/mock/profile_test.go
git commit -m "feat(mock): RenderConfig — profile lookup + override merge (RUN-38195)"
```

---

## Task 15: Piece A — `resources.go` (DaemonSet + ConfigMap builders)

**Files:**
- Create: `internal/status-updater/controllers/mock/resources.go`
- Create: `internal/status-updater/controllers/mock/resources_test.go`

Mirrors upstream `nvml-mock v0.1.0`'s `templates/daemonset.yaml` — see `https://github.com/NVIDIA/k8s-test-infra/blob/v0.1.0/deployments/nvml-mock/helm/nvml-mock/templates/daemonset.yaml`. The shape (~55 lines) is reproduced inline below; **at each upstream version bump, re-read the upstream template and reconcile**.

- [ ] **Step 1: Write the failing tests**

Create `internal/status-updater/controllers/mock/resources_test.go`:

```go
package mock

import (
	"testing"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestBuildConfigMap(t *testing.T) {
	cm := BuildConfigMap("ns", "training", []byte("driver_version: \"550\""))

	assert.Equal(t, "nvml-mock-training", cm.Name)
	assert.Equal(t, "ns", cm.Namespace)
	assert.Equal(t, constants.LabelManagedByValue, cm.Labels[constants.LabelManagedBy])
	assert.Equal(t, constants.ComponentNvmlMock, cm.Labels[constants.LabelComponent])
	assert.Equal(t, "training", cm.Labels[constants.LabelPool])
	assert.Equal(t, "driver_version: \"550\"", cm.Data["config.yaml"])
}

func TestBuildDaemonSet_Shape(t *testing.T) {
	ds := BuildDaemonSet(BuildDaemonSetParams{
		Namespace:        "ns",
		Pool:             "training",
		NodePoolLabelKey: "run.ai/simulated-gpu-node-pool",
		Image:            "ghcr.io/nvidia/nvml-mock:v0.1.0",
		ImagePullPolicy:  corev1.PullIfNotPresent,
		ConfigHash:       "abc123",
	})

	require.Equal(t, "nvml-mock-training", ds.Name)
	assert.Equal(t, "ns", ds.Namespace)
	assert.Equal(t, constants.LabelManagedByValue, ds.Labels[constants.LabelManagedBy])
	assert.Equal(t, constants.ComponentNvmlMock, ds.Labels[constants.LabelComponent])
	assert.Equal(t, "training", ds.Labels[constants.LabelPool])

	pod := ds.Spec.Template
	assert.Equal(t, "training", pod.Spec.NodeSelector["run.ai/simulated-gpu-node-pool"])
	assert.Equal(t, "nvml-mock", pod.Spec.ServiceAccountName)
	assert.Equal(t, "abc123", pod.Annotations["fake-gpu-operator/config-hash"])

	require.Len(t, pod.Spec.Containers, 1)
	c := pod.Spec.Containers[0]
	assert.Equal(t, "nvml-mock", c.Name)
	assert.Equal(t, "ghcr.io/nvidia/nvml-mock:v0.1.0", c.Image)
	assert.Equal(t, corev1.PullIfNotPresent, c.ImagePullPolicy)
	require.NotNil(t, c.SecurityContext)
	require.NotNil(t, c.SecurityContext.Privileged)
	assert.True(t, *c.SecurityContext.Privileged)

	envNames := map[string]bool{}
	for _, e := range c.Env {
		envNames[e.Name] = true
	}
	assert.True(t, envNames["GPU_COUNT"])
	assert.True(t, envNames["DRIVER_VERSION"])
	assert.True(t, envNames["NODE_NAME"])

	mountPaths := map[string]bool{}
	for _, m := range c.VolumeMounts {
		mountPaths[m.MountPath] = true
	}
	assert.True(t, mountPaths["/host/var/lib/nvml-mock"])
	assert.True(t, mountPaths["/config"])
	assert.True(t, mountPaths["/host/var/run/cdi"])
	assert.True(t, mountPaths["/host/run/nvidia"])
}

func TestBuildDaemonSet_ConfigMapVolumePointsAtPerPoolCM(t *testing.T) {
	ds := BuildDaemonSet(BuildDaemonSetParams{
		Namespace: "ns", Pool: "training",
		NodePoolLabelKey: "k", Image: "img", ImagePullPolicy: corev1.PullAlways,
		ConfigHash: "x",
	})
	for _, v := range ds.Spec.Template.Spec.Volumes {
		if v.Name == "gpu-config" {
			require.NotNil(t, v.ConfigMap)
			assert.Equal(t, "nvml-mock-training", v.ConfigMap.Name)
			return
		}
	}
	t.Fatal("gpu-config volume not found")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/status-updater/controllers/mock/... -run "BuildConfigMap|BuildDaemonSet" -v`
Expected: fail with `undefined: BuildConfigMap` / `undefined: BuildDaemonSet`.

- [ ] **Step 3: Implement `resources.go`**

Create `internal/status-updater/controllers/mock/resources.go`:

```go
package mock

import (
	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	// ConfigHashAnnotation is stamped on the DaemonSet pod template so a
	// ConfigMap content change forces a pod rollout.
	ConfigHashAnnotation = "fake-gpu-operator/config-hash"

	// configKey is the data key in the per-pool ConfigMap. Mirrors
	// nvml-mock's chart, which mounts the file at /config/config.yaml.
	configKey = "config.yaml"
)

// resourceName produces the per-pool resource name shared by the DaemonSet
// and the ConfigMap (also used as the diff key).
func resourceName(pool string) string {
	return "nvml-mock-" + pool
}

// managedLabels returns the label set every controller-owned resource carries.
func managedLabels(pool string) map[string]string {
	return map[string]string{
		constants.LabelManagedBy: constants.LabelManagedByValue,
		constants.LabelComponent: constants.ComponentNvmlMock,
		constants.LabelPool:      pool,
	}
}

// BuildConfigMap produces the per-pool nvml-mock ConfigMap whose `config.yaml`
// key is mounted into the DaemonSet at /config/config.yaml.
func BuildConfigMap(namespace, pool string, configYAML []byte) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName(pool),
			Namespace: namespace,
			Labels:    managedLabels(pool),
		},
		Data: map[string]string{
			configKey: string(configYAML),
		},
	}
}

// BuildDaemonSetParams captures everything BuildDaemonSet needs.
type BuildDaemonSetParams struct {
	Namespace        string
	Pool             string
	NodePoolLabelKey string
	Image            string
	ImagePullPolicy  corev1.PullPolicy
	// ConfigHash is the SHA of the ConfigMap's config.yaml content; stamped
	// on the pod template so changes force a rolling restart.
	ConfigHash string
}

// BuildDaemonSet produces the per-pool nvml-mock DaemonSet. Mirrors upstream
// nvml-mock v0.1.0 templates/daemonset.yaml.
func BuildDaemonSet(p BuildDaemonSetParams) *appsv1.DaemonSet {
	labels := managedLabels(p.Pool)
	name := resourceName(p.Pool)

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: p.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: map[string]string{ConfigHashAnnotation: p.ConfigHash},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "nvml-mock",
					NodeSelector:       map[string]string{p.NodePoolLabelKey: p.Pool},
					Containers: []corev1.Container{{
						Name:            "nvml-mock",
						Image:           p.Image,
						ImagePullPolicy: p.ImagePullPolicy,
						SecurityContext: &corev1.SecurityContext{Privileged: ptr.To(true)},
						Command:         []string{"/scripts/entrypoint.sh"},
						Lifecycle: &corev1.Lifecycle{
							PreStop: &corev1.LifecycleHandler{
								Exec: &corev1.ExecAction{Command: []string{"/scripts/cleanup.sh"}},
							},
						},
						Env: []corev1.EnvVar{
							{
								Name: "GPU_COUNT",
								ValueFrom: &corev1.EnvVarSource{
									ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: name},
										Key:                  "GPU_COUNT",
										Optional:             ptr.To(true),
									},
								},
							},
							{
								Name: "DRIVER_VERSION",
								ValueFrom: &corev1.EnvVarSource{
									ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: name},
										Key:                  "DRIVER_VERSION",
										Optional:             ptr.To(true),
									},
								},
							},
							{
								Name:      "NODE_NAME",
								ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "spec.nodeName"}},
							},
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "host-nvml-mock", MountPath: "/host/var/lib/nvml-mock"},
							{Name: "gpu-config", MountPath: "/config"},
							{Name: "host-cdi", MountPath: "/host/var/run/cdi"},
							{Name: "host-run-nvidia", MountPath: "/host/run/nvidia"},
						},
					}},
					Volumes: []corev1.Volume{
						{
							Name: "host-nvml-mock",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/lib/nvml-mock",
									Type: ptr.To(corev1.HostPathDirectoryOrCreate),
								},
							},
						},
						{
							Name: "gpu-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: name},
								},
							},
						},
						{
							Name: "host-cdi",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/var/run/cdi",
									Type: ptr.To(corev1.HostPathDirectoryOrCreate),
								},
							},
						},
						{
							Name: "host-run-nvidia",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/run/nvidia",
									Type: ptr.To(corev1.HostPathDirectoryOrCreate),
								},
							},
						},
					},
				},
			},
		},
	}
}
```

**Note on env vars:** upstream nvml-mock derives `GPU_COUNT` and `DRIVER_VERSION` via Helm template helpers from the profile values. Since we render the full profile YAML into the ConfigMap and mount it at `/config/config.yaml`, the binary already has access to those values. The env-var entries above use `optional: true` ConfigMapKeyRef as a soft handoff in case future nvml-mock versions read them from env — they're harmless when absent. **Verify against the upstream binary at version-bump time** that env-injected values aren't required.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/status-updater/controllers/mock/... -run "BuildConfigMap|BuildDaemonSet" -v`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/status-updater/controllers/mock/resources.go \
        internal/status-updater/controllers/mock/resources_test.go
git commit -m "feat(mock): nvml-mock DaemonSet + ConfigMap builders (RUN-38195)"
```

---

## Task 16: Piece A — `diff.go`

**Files:**
- Create: `internal/status-updater/controllers/mock/diff.go`
- Create: `internal/status-updater/controllers/mock/diff_test.go`

Filter actual state by `managed-by=fake-gpu-operator` + `component=nvml-mock`; produce two diffs.

- [ ] **Step 1: Write the failing tests**

Create `internal/status-updater/controllers/mock/diff_test.go`:

```go
package mock

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func newDS(name, image, configHash string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{ConfigHashAnnotation: configHash},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Image: image}},
				},
			},
		},
	}
}

func newCM(name, body string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Data:       map[string]string{"config.yaml": body},
	}
}

func TestDiffDaemonSets_Create(t *testing.T) {
	desired := []runtime.Object{newDS("nvml-mock-a", "img:1", "h1")}
	d := DiffDaemonSets(desired, nil)
	require.Len(t, d.ToCreate, 1)
	assert.Equal(t, "nvml-mock-a", d.ToCreate[0].Name)
}

func TestDiffDaemonSets_NoOp(t *testing.T) {
	desired := []runtime.Object{newDS("nvml-mock-a", "img:1", "h1")}
	actual := []appsv1.DaemonSet{*newDS("nvml-mock-a", "img:1", "h1")}
	d := DiffDaemonSets(desired, actual)
	assert.Empty(t, d.ToCreate)
	assert.Empty(t, d.ToUpdate)
	assert.Empty(t, d.ToDelete)
}

func TestDiffDaemonSets_UpdateOnImageChange(t *testing.T) {
	desired := []runtime.Object{newDS("nvml-mock-a", "img:2", "h1")}
	existing := newDS("nvml-mock-a", "img:1", "h1")
	existing.ResourceVersion = "42"
	d := DiffDaemonSets(desired, []appsv1.DaemonSet{*existing})
	require.Len(t, d.ToUpdate, 1)
	assert.Equal(t, "42", d.ToUpdate[0].ResourceVersion, "ResourceVersion copied for optimistic concurrency")
}

func TestDiffDaemonSets_UpdateOnConfigHashChange(t *testing.T) {
	desired := []runtime.Object{newDS("nvml-mock-a", "img:1", "h2")}
	existing := newDS("nvml-mock-a", "img:1", "h1")
	existing.ResourceVersion = "7"
	d := DiffDaemonSets(desired, []appsv1.DaemonSet{*existing})
	require.Len(t, d.ToUpdate, 1)
	assert.Equal(t, "h2", d.ToUpdate[0].Spec.Template.Annotations[ConfigHashAnnotation])
}

func TestDiffDaemonSets_Delete(t *testing.T) {
	actual := []appsv1.DaemonSet{*newDS("nvml-mock-old", "img:1", "h1")}
	d := DiffDaemonSets(nil, actual)
	require.Len(t, d.ToDelete, 1)
	assert.Equal(t, "nvml-mock-old", d.ToDelete[0].Name)
}

func TestDiffConfigMaps_Create(t *testing.T) {
	desired := []runtime.Object{newCM("nvml-mock-a", "body1")}
	d := DiffConfigMaps(desired, nil)
	require.Len(t, d.ToCreate, 1)
}

func TestDiffConfigMaps_UpdateOnDataChange(t *testing.T) {
	desired := []runtime.Object{newCM("nvml-mock-a", "body2")}
	existing := newCM("nvml-mock-a", "body1")
	existing.ResourceVersion = "13"
	d := DiffConfigMaps(desired, []corev1.ConfigMap{*existing})
	require.Len(t, d.ToUpdate, 1)
	assert.Equal(t, "13", d.ToUpdate[0].ResourceVersion)
	assert.Equal(t, "body2", d.ToUpdate[0].Data["config.yaml"])
}

func TestDiffConfigMaps_NoOp(t *testing.T) {
	desired := []runtime.Object{newCM("nvml-mock-a", "body1")}
	actual := []corev1.ConfigMap{*newCM("nvml-mock-a", "body1")}
	d := DiffConfigMaps(desired, actual)
	assert.Empty(t, d.ToCreate)
	assert.Empty(t, d.ToUpdate)
	assert.Empty(t, d.ToDelete)
}

func TestDiffConfigMaps_Delete(t *testing.T) {
	actual := []corev1.ConfigMap{*newCM("nvml-mock-old", "body1")}
	d := DiffConfigMaps(nil, actual)
	require.Len(t, d.ToDelete, 1)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/status-updater/controllers/mock/... -run "Diff" -v`
Expected: all fail with `undefined: DiffDaemonSets` / `undefined: DiffConfigMaps`.

- [ ] **Step 3: Implement `diff.go`**

Create `internal/status-updater/controllers/mock/diff.go`:

```go
package mock

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DaemonSetDiff partitions name-keyed DaemonSets into Create/Update/Delete.
type DaemonSetDiff struct {
	ToCreate []*appsv1.DaemonSet
	ToUpdate []*appsv1.DaemonSet
	ToDelete []appsv1.DaemonSet
}

// ConfigMapDiff partitions name-keyed ConfigMaps. ToUpdate carries
// ResourceVersion-stamped objects ready to send to Update.
type ConfigMapDiff struct {
	ToCreate []*corev1.ConfigMap
	ToUpdate []*corev1.ConfigMap
	ToDelete []corev1.ConfigMap
}

// DiffDaemonSets compares desired vs actual. A DaemonSet that exists in both
// triggers an Update iff its first container image differs OR its config-hash
// annotation differs. ResourceVersion is copied from actual into desired
// before Update (optimistic concurrency requirement).
func DiffDaemonSets(desired []runtime.Object, actual []appsv1.DaemonSet) DaemonSetDiff {
	desiredByName := make(map[string]*appsv1.DaemonSet)
	for _, obj := range desired {
		if ds, ok := obj.(*appsv1.DaemonSet); ok {
			desiredByName[ds.Name] = ds
		}
	}
	actualByName := make(map[string]appsv1.DaemonSet, len(actual))
	for _, ds := range actual {
		actualByName[ds.Name] = ds
	}

	var d DaemonSetDiff
	for name, want := range desiredByName {
		have, exists := actualByName[name]
		if !exists {
			d.ToCreate = append(d.ToCreate, want)
			continue
		}
		if daemonsetNeedsUpdate(want, &have) {
			want.ResourceVersion = have.ResourceVersion
			d.ToUpdate = append(d.ToUpdate, want)
		}
	}
	for _, have := range actual {
		if _, stillDesired := desiredByName[have.Name]; !stillDesired {
			d.ToDelete = append(d.ToDelete, have)
		}
	}
	return d
}

// daemonsetNeedsUpdate returns true iff the first container image OR the
// config-hash annotation differs. Other fields are sourced from the
// status-updater pod's env/values or hardcoded in resources.go and only
// change across a status-updater rollout — so a CM-driven reconcile cannot
// observe a difference there.
func daemonsetNeedsUpdate(want, have *appsv1.DaemonSet) bool {
	if len(want.Spec.Template.Spec.Containers) == 0 || len(have.Spec.Template.Spec.Containers) == 0 {
		return true
	}
	if want.Spec.Template.Spec.Containers[0].Image != have.Spec.Template.Spec.Containers[0].Image {
		return true
	}
	return want.Spec.Template.Annotations[ConfigHashAnnotation] != have.Spec.Template.Annotations[ConfigHashAnnotation]
}

// DiffConfigMaps compares desired vs actual ConfigMaps. Update fires when
// data["config.yaml"] differs.
func DiffConfigMaps(desired []runtime.Object, actual []corev1.ConfigMap) ConfigMapDiff {
	desiredByName := make(map[string]*corev1.ConfigMap)
	for _, obj := range desired {
		if cm, ok := obj.(*corev1.ConfigMap); ok {
			desiredByName[cm.Name] = cm
		}
	}
	actualByName := make(map[string]corev1.ConfigMap, len(actual))
	for _, cm := range actual {
		actualByName[cm.Name] = cm
	}

	var d ConfigMapDiff
	for name, want := range desiredByName {
		have, exists := actualByName[name]
		if !exists {
			d.ToCreate = append(d.ToCreate, want)
			continue
		}
		if want.Data["config.yaml"] != have.Data["config.yaml"] {
			want.ResourceVersion = have.ResourceVersion
			d.ToUpdate = append(d.ToUpdate, want)
		}
	}
	for _, have := range actual {
		if _, stillDesired := desiredByName[have.Name]; !stillDesired {
			d.ToDelete = append(d.ToDelete, have)
		}
	}
	return d
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/status-updater/controllers/mock/... -run "Diff" -v`
Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/status-updater/controllers/mock/diff.go \
        internal/status-updater/controllers/mock/diff_test.go
git commit -m "feat(mock): DaemonSet + ConfigMap diff partitioning (RUN-38195)"
```

---

## Task 17: Piece A — `desired_state.go`

**Files:**
- Create: `internal/status-updater/controllers/mock/desired_state.go`
- Create: `internal/status-updater/controllers/mock/desired_state_test.go`

Pure function: ClusterConfig → []runtime.Object, iterating mock pools.

- [ ] **Step 1: Write the failing tests**

Create `internal/status-updater/controllers/mock/desired_state_test.go`:

```go
package mock

import (
	"testing"

	commonprofile "github.com/run-ai/fake-gpu-operator/internal/common/profile"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func mkProfileCM(name, ns string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    map[string]string{commonprofile.LabelGpuProfile: "true"},
		},
		Data: map[string]string{
			commonprofile.CmProfileKey: `system:
  driver_version: "550"`,
		},
	}
}

func TestComputeDesiredState_OnlyMockPools(t *testing.T) {
	kube := fake.NewSimpleClientset(mkProfileCM("gpu-profile-a100", "ns"))
	cfg := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools: map[string]topology.NodePoolConfig{
			"fake-pool":    {Gpu: topology.GpuConfig{Backend: "fake", Profile: "a100"}},
			"mock-train":   {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
			"mock-inference": {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
		},
	}
	params := ReconcileParams{
		Namespace:       "ns",
		Image:           "ghcr.io/nvidia/nvml-mock:v0.1.0",
		ImagePullPolicy: corev1.PullIfNotPresent,
	}

	objs, err := ComputeDesiredState(kube, cfg, params)
	require.NoError(t, err)

	// 2 mock pools × (1 DS + 1 CM) = 4 resources; fake pool produces 0.
	assert.Len(t, objs, 4, "exactly two DS+CM pairs for the two mock pools")

	dsNames, cmNames := map[string]bool{}, map[string]bool{}
	for _, o := range objs {
		switch r := o.(type) {
		case *appsv1.DaemonSet:
			dsNames[r.Name] = true
		case *corev1.ConfigMap:
			cmNames[r.Name] = true
		}
	}
	assert.True(t, dsNames["nvml-mock-mock-train"])
	assert.True(t, dsNames["nvml-mock-mock-inference"])
	assert.True(t, cmNames["nvml-mock-mock-train"])
	assert.True(t, cmNames["nvml-mock-mock-inference"])
	assert.False(t, dsNames["nvml-mock-fake-pool"])
}

func TestComputeDesiredState_DeterministicOrder(t *testing.T) {
	kube := fake.NewSimpleClientset(mkProfileCM("gpu-profile-a100", "ns"))
	cfg := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools: map[string]topology.NodePoolConfig{
			"zzz": {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
			"aaa": {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
			"mmm": {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
		},
	}
	params := ReconcileParams{Namespace: "ns", Image: "img:t"}

	objs, err := ComputeDesiredState(kube, cfg, params)
	require.NoError(t, err)

	// Pool order should be sorted: aaa, mmm, zzz. Each pool emits CM then DS.
	require.GreaterOrEqual(t, len(objs), 6)
	cm0 := objs[0].(*corev1.ConfigMap)
	cm2 := objs[2].(*corev1.ConfigMap)
	cm4 := objs[4].(*corev1.ConfigMap)
	assert.Equal(t, "nvml-mock-aaa", cm0.Name)
	assert.Equal(t, "nvml-mock-mmm", cm2.Name)
	assert.Equal(t, "nvml-mock-zzz", cm4.Name)
}

func TestComputeDesiredState_PropagatesProfileError(t *testing.T) {
	// No profile CMs in cluster → load fails for the mock pool.
	kube := fake.NewSimpleClientset()
	cfg := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools: map[string]topology.NodePoolConfig{
			"mock-pool": {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
		},
	}
	_, err := ComputeDesiredState(kube, cfg, ReconcileParams{Namespace: "ns"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock-pool")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/status-updater/controllers/mock/... -run ComputeDesiredState -v`
Expected: fail with `undefined: ComputeDesiredState`, `undefined: ReconcileParams`.

- [ ] **Step 3: Implement `desired_state.go`**

Create `internal/status-updater/controllers/mock/desired_state.go`:

```go
package mock

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

// ReconcileParams holds the controller's runtime configuration.
type ReconcileParams struct {
	Namespace       string
	// Image is the full nvml-mock image reference (e.g.
	// "ghcr.io/nvidia/nvml-mock:v0.1.0"), supplied via the NVML_MOCK_IMAGE
	// env var the chart plumbs from values.yaml's nvmlMock.image.repository
	// + tag. Every per-pool DaemonSet gets the same image — there's no
	// per-pool image override mechanism by design.
	Image           string
	ImagePullPolicy corev1.PullPolicy
}

// ComputeDesiredState walks all mock pools in the topology config and produces
// the per-pool ConfigMap + DaemonSet pairs the controller should ensure exist.
// Pools are iterated in sorted order for deterministic output. Each pool emits
// the ConfigMap first, then its DaemonSet (so callers iterating in order create
// the CM before the DS, though the reconciler does this explicitly via
// separate diff stages).
func ComputeDesiredState(
	kube kubernetes.Interface,
	cfg *topology.ClusterConfig,
	params ReconcileParams,
) ([]runtime.Object, error) {
	poolNames := make([]string, 0, len(cfg.NodePools))
	for name := range cfg.NodePools {
		poolNames = append(poolNames, name)
	}
	sort.Strings(poolNames)

	pullPolicy := params.ImagePullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullAlways
	}

	var resources []runtime.Object
	for _, name := range poolNames {
		pool := cfg.NodePools[name]
		if pool.Gpu.Backend != constants.BackendMock {
			continue
		}

		configYAML, err := RenderConfig(kube, params.Namespace, pool.Gpu)
		if err != nil {
			return nil, fmt.Errorf("rendering config for pool %q: %w", name, err)
		}

		cm := BuildConfigMap(params.Namespace, name, configYAML)
		ds := BuildDaemonSet(BuildDaemonSetParams{
			Namespace:        params.Namespace,
			Pool:             name,
			NodePoolLabelKey: cfg.NodePoolLabelKey,
			Image:            params.Image,
			ImagePullPolicy:  pullPolicy,
			ConfigHash:       configHash(configYAML),
		})

		resources = append(resources, cm, ds)
	}
	return resources, nil
}

// configHash produces a short hex SHA-256 of the rendered config YAML.
// Stamped on the DaemonSet pod template via the ConfigHashAnnotation so a CM
// content change forces a rolling restart.
func configHash(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/status-updater/controllers/mock/... -run ComputeDesiredState -v`
Expected: all `ComputeDesiredState_*` cases pass.

- [ ] **Step 5: Commit**

```bash
git add internal/status-updater/controllers/mock/desired_state.go \
        internal/status-updater/controllers/mock/desired_state_test.go \
        internal/status-updater/controllers/component/images.go
git commit -m "feat(mock): ComputeDesiredState + nvmlMock image resolution (RUN-38195)"
```

---

## Task 18: Piece A — `reconciler.go`

**Files:**
- Create: `internal/status-updater/controllers/mock/reconciler.go`
- Create: `internal/status-updater/controllers/mock/reconciler_test.go`

The orchestrator: read topology CM → ComputeDesiredState → diff against label-selected actuals → apply.

- [ ] **Step 1: Write the failing tests**

Create `internal/status-updater/controllers/mock/reconciler_test.go`:

```go
package mock

import (
	"context"
	"testing"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	commonprofile "github.com/run-ai/fake-gpu-operator/internal/common/profile"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func makeTopologyCM(t *testing.T, ns string, cfg *topology.ClusterConfig) *corev1.ConfigMap {
	t.Helper()
	body, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "topology", Namespace: ns},
		Data:       map[string]string{"topology.yml": string(body)},
	}
}

func makeProfileCM(name, ns string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    map[string]string{commonprofile.LabelGpuProfile: "true"},
		},
		Data: map[string]string{commonprofile.CmProfileKey: `system: { driver_version: "550" }`},
	}
}

func TestReconcile_Empty_NoOp(t *testing.T) {
	cfg := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools:        map[string]topology.NodePoolConfig{},
	}
	kube := fake.NewSimpleClientset(makeTopologyCM(t, "ns", cfg))
	r := NewReconciler(kube, ReconcileParams{Namespace: "ns"})
	require.NoError(t, r.Reconcile(context.Background()))
}

func TestReconcile_PoolAdded_CreatesResources(t *testing.T) {
	cfg := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools: map[string]topology.NodePoolConfig{
			"train": {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
		},
	}
	kube := fake.NewSimpleClientset(
		makeTopologyCM(t, "ns", cfg),
		makeProfileCM("gpu-profile-a100", "ns"),
	)
	r := NewReconciler(kube, ReconcileParams{
		Namespace: "ns", Image: "ghcr.io/nvidia/nvml-mock:v0.1.0",
	})
	require.NoError(t, r.Reconcile(context.Background()))

	ds, err := kube.AppsV1().DaemonSets("ns").Get(context.Background(), "nvml-mock-train", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, constants.LabelManagedByValue, ds.Labels[constants.LabelManagedBy])

	cm, err := kube.CoreV1().ConfigMaps("ns").Get(context.Background(), "nvml-mock-train", metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotEmpty(t, cm.Data["config.yaml"])
}

func TestReconcile_PoolRemoved_DeletesResources(t *testing.T) {
	cfg := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools:        map[string]topology.NodePoolConfig{}, // no pools
	}
	preexistingDS := BuildDaemonSet(BuildDaemonSetParams{
		Namespace: "ns", Pool: "old", NodePoolLabelKey: "k",
		Image: "img", ImagePullPolicy: corev1.PullAlways, ConfigHash: "h",
	})
	preexistingCM := BuildConfigMap("ns", "old", []byte("body"))

	kube := fake.NewSimpleClientset(
		makeTopologyCM(t, "ns", cfg),
		preexistingDS,
		preexistingCM,
	)
	r := NewReconciler(kube, ReconcileParams{Namespace: "ns"})
	require.NoError(t, r.Reconcile(context.Background()))

	_, err := kube.AppsV1().DaemonSets("ns").Get(context.Background(), "nvml-mock-old", metav1.GetOptions{})
	assert.True(t, isNotFound(err), "DaemonSet should have been deleted")

	_, err = kube.CoreV1().ConfigMaps("ns").Get(context.Background(), "nvml-mock-old", metav1.GetOptions{})
	assert.True(t, isNotFound(err), "ConfigMap should have been deleted")
}

func TestReconcile_OverrideChange_UpdatesConfigMapAndStampsDaemonSet(t *testing.T) {
	cfg1 := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools: map[string]topology.NodePoolConfig{
			"train": {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
		},
	}
	kube := fake.NewSimpleClientset(
		makeTopologyCM(t, "ns", cfg1),
		makeProfileCM("gpu-profile-a100", "ns"),
	)
	r := NewReconciler(kube, ReconcileParams{
		Namespace: "ns", Image: "img:t",
	})
	require.NoError(t, r.Reconcile(context.Background()))

	ds1, err := kube.AppsV1().DaemonSets("ns").Get(context.Background(), "nvml-mock-train", metav1.GetOptions{})
	require.NoError(t, err)
	hash1 := ds1.Spec.Template.Annotations[ConfigHashAnnotation]

	// Override the driver_version. CM body must change → hash must change.
	cfg2 := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools: map[string]topology.NodePoolConfig{
			"train": {Gpu: topology.GpuConfig{
				Backend: "mock", Profile: "a100",
				Overrides: map[string]interface{}{"system": map[string]interface{}{"driver_version": "999"}},
			}},
		},
	}
	updatedTopologyCM := makeTopologyCM(t, "ns", cfg2)
	updatedTopologyCM.ResourceVersion = "999"
	_, err = kube.CoreV1().ConfigMaps("ns").Update(context.Background(), updatedTopologyCM, metav1.UpdateOptions{})
	require.NoError(t, err)
	require.NoError(t, r.Reconcile(context.Background()))

	ds2, err := kube.AppsV1().DaemonSets("ns").Get(context.Background(), "nvml-mock-train", metav1.GetOptions{})
	require.NoError(t, err)
	hash2 := ds2.Spec.Template.Annotations[ConfigHashAnnotation]
	assert.NotEqual(t, hash1, hash2, "config-hash must change when override changes")
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() != "" && (contains(err.Error(), "not found") || contains(err.Error(), "NotFound"))
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/status-updater/controllers/mock/... -run "Reconcile" -v`
Expected: fail with `undefined: NewReconciler` etc.

- [ ] **Step 3: Implement `reconciler.go`**

Create `internal/status-updater/controllers/mock/reconciler.go`:

```go
package mock

import (
	"context"
	"fmt"
	"log"

	"github.com/run-ai/fake-gpu-operator/internal/common/constants"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

// Reconciler runs the desired-state pipeline for mock-pool resources.
type Reconciler struct {
	kube   kubernetes.Interface
	params ReconcileParams
}

// NewReconciler constructs a Reconciler. ReconcileParams.Namespace must be
// the namespace of the topology ConfigMap (and where per-pool resources
// are created).
func NewReconciler(kube kubernetes.Interface, params ReconcileParams) *Reconciler {
	return &Reconciler{kube: kube, params: params}
}

// Reconcile performs one full pipeline pass: read topology, compute desired
// state, diff against actual, apply.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	cfg, err := r.readTopologyConfig(ctx)
	if err != nil {
		return fmt.Errorf("reading topology config: %w", err)
	}

	desired, err := ComputeDesiredState(r.kube, cfg, r.params)
	if err != nil {
		return fmt.Errorf("computing desired state: %w", err)
	}

	if err := r.reconcileDaemonSets(ctx, desired); err != nil {
		return fmt.Errorf("reconciling DaemonSets: %w", err)
	}
	if err := r.reconcileConfigMaps(ctx, desired); err != nil {
		return fmt.Errorf("reconciling ConfigMaps: %w", err)
	}
	return nil
}

func (r *Reconciler) readTopologyConfig(ctx context.Context) (*topology.ClusterConfig, error) {
	cm, err := r.kube.CoreV1().ConfigMaps(r.params.Namespace).
		Get(ctx, "topology", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return topology.FromClusterConfigCM(cm)
}

func (r *Reconciler) managedSelector() string {
	return constants.LabelManagedBy + "=" + constants.LabelManagedByValue +
		"," + constants.LabelComponent + "=" + constants.ComponentNvmlMock
}

func (r *Reconciler) reconcileDaemonSets(ctx context.Context, desired []runtime.Object) error {
	actual, err := r.kube.AppsV1().DaemonSets(r.params.Namespace).
		List(ctx, metav1.ListOptions{LabelSelector: r.managedSelector()})
	if err != nil {
		return fmt.Errorf("listing managed DaemonSets: %w", err)
	}
	d := DiffDaemonSets(desired, actual.Items)

	for _, ds := range d.ToCreate {
		log.Printf("Creating DaemonSet %s/%s", ds.Namespace, ds.Name)
		if _, err := r.kube.AppsV1().DaemonSets(r.params.Namespace).
			Create(ctx, ds, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating DaemonSet %s: %w", ds.Name, err)
		}
	}
	for _, ds := range d.ToUpdate {
		log.Printf("Updating DaemonSet %s/%s", ds.Namespace, ds.Name)
		if _, err := r.kube.AppsV1().DaemonSets(r.params.Namespace).
			Update(ctx, ds, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("updating DaemonSet %s: %w", ds.Name, err)
		}
	}
	for _, ds := range d.ToDelete {
		log.Printf("Deleting DaemonSet %s/%s", ds.Namespace, ds.Name)
		if err := r.kube.AppsV1().DaemonSets(r.params.Namespace).
			Delete(ctx, ds.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("deleting DaemonSet %s: %w", ds.Name, err)
		}
	}
	return nil
}

func (r *Reconciler) reconcileConfigMaps(ctx context.Context, desired []runtime.Object) error {
	actual, err := r.kube.CoreV1().ConfigMaps(r.params.Namespace).
		List(ctx, metav1.ListOptions{LabelSelector: r.managedSelector()})
	if err != nil {
		return fmt.Errorf("listing managed ConfigMaps: %w", err)
	}
	d := DiffConfigMaps(desired, actual.Items)

	for _, cm := range d.ToCreate {
		log.Printf("Creating ConfigMap %s/%s", cm.Namespace, cm.Name)
		if _, err := r.kube.CoreV1().ConfigMaps(r.params.Namespace).
			Create(ctx, cm, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("creating ConfigMap %s: %w", cm.Name, err)
		}
	}
	for _, cm := range d.ToUpdate {
		log.Printf("Updating ConfigMap %s/%s", cm.Namespace, cm.Name)
		if _, err := r.kube.CoreV1().ConfigMaps(r.params.Namespace).
			Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("updating ConfigMap %s: %w", cm.Name, err)
		}
	}
	for _, cm := range d.ToDelete {
		log.Printf("Deleting ConfigMap %s/%s", cm.Namespace, cm.Name)
		if err := r.kube.CoreV1().ConfigMaps(r.params.Namespace).
			Delete(ctx, cm.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("deleting ConfigMap %s: %w", cm.Name, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/status-updater/controllers/mock/... -run "Reconcile" -v`
Expected: all four reconcile cases pass.

- [ ] **Step 5: Commit**

```bash
git add internal/status-updater/controllers/mock/reconciler.go \
        internal/status-updater/controllers/mock/reconciler_test.go
git commit -m "feat(mock): Reconciler — read CM, diff, apply (RUN-38195)"
```

---

## Task 19: Piece A — `controller.go`

**Files:**
- Create: `internal/status-updater/controllers/mock/controller.go`
- Create: `internal/status-updater/controllers/mock/controller_test.go`

Sets up a `SharedIndexInformer` watching the topology CM and triggers `Reconcile` on Add/Update.

- [ ] **Step 1: Write the failing tests**

Create `internal/status-updater/controllers/mock/controller_test.go`:

```go
package mock

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers"
	commonprofile "github.com/run-ai/fake-gpu-operator/internal/common/profile"
	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestController_ImplementsInterface(t *testing.T) {
	var _ controllers.Interface = (*MockController)(nil)
}

func TestController_ReconcilesOnTopologyChange(t *testing.T) {
	cfg := &topology.ClusterConfig{
		NodePoolLabelKey: "k",
		NodePools: map[string]topology.NodePoolConfig{
			"train": {Gpu: topology.GpuConfig{Backend: "mock", Profile: "a100"}},
		},
	}
	body, err := yaml.Marshal(cfg)
	require.NoError(t, err)

	kube := fake.NewSimpleClientset(
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "topology", Namespace: "ns"},
			Data:       map[string]string{"topology.yml": string(body)},
		},
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: "gpu-profile-a100", Namespace: "ns",
				Labels: map[string]string{commonprofile.LabelGpuProfile: "true"},
			},
			Data: map[string]string{commonprofile.CmProfileKey: `system: { driver_version: "550" }`},
		},
	)

	c := NewMockController(kube, ReconcileParams{
		Namespace: "ns", Image: "img:t",
	})

	stopCh := make(chan struct{})
	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.Run(stopCh)
	}()

	// Wait for the informer to fire AddFunc → Reconcile → DaemonSet appears.
	require.Eventually(t, func() bool {
		_, err := kube.AppsV1().DaemonSets("ns").
			Get(context.Background(), "nvml-mock-train", metav1.GetOptions{})
		return err == nil
	}, 3*time.Second, 50*time.Millisecond)

	close(stopCh)
	wg.Wait()

	ds, err := kube.AppsV1().DaemonSets("ns").
		Get(context.Background(), "nvml-mock-train", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "nvml-mock-train", ds.Name)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/status-updater/controllers/mock/... -run Controller -v`
Expected: fail with `undefined: NewMockController`, `undefined: MockController`.

- [ ] **Step 3: Implement `controller.go`**

Create `internal/status-updater/controllers/mock/controller.go`:

```go
package mock

import (
	"context"
	"log"
	"time"

	"github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// MockController watches the cluster topology ConfigMap and reconciles
// per-pool nvml-mock DaemonSets and ConfigMaps.
type MockController struct {
	informer   cache.SharedIndexInformer
	reconciler *Reconciler
}

var _ controllers.Interface = (*MockController)(nil)

// NewMockController constructs a controller and wires its informer.
func NewMockController(kube kubernetes.Interface, params ReconcileParams) *MockController {
	factory := informers.NewSharedInformerFactoryWithOptions(
		kube,
		30*time.Second,
		informers.WithNamespace(params.Namespace),
		informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.FieldSelector = fields.OneTermEqualSelector("metadata.name", "topology").String()
		}),
	)
	informer := factory.Core().V1().ConfigMaps().Informer()

	c := &MockController{
		informer:   informer,
		reconciler: NewReconciler(kube, params),
	}

	if _, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { c.handleEvent(obj) },
		UpdateFunc: func(_, newObj interface{}) { c.handleEvent(newObj) },
	}); err != nil {
		log.Fatalf("mock controller: failed to add event handler: %v", err)
	}

	return c
}

// Run blocks until stopCh is closed.
func (c *MockController) Run(stopCh <-chan struct{}) {
	log.Println("Starting mock controller")
	c.informer.Run(stopCh)
}

func (c *MockController) handleEvent(obj interface{}) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}
	if cm.Name != "topology" {
		return
	}
	log.Printf("mock controller: topology CM event, reconciling")
	if err := c.reconciler.Reconcile(context.Background()); err != nil {
		log.Printf("mock controller: reconcile error: %v", err)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/status-updater/controllers/mock/... -count=1 -v`
Expected: all tests in the package pass — controller, reconciler, desired_state, diff, resources, profile.

- [ ] **Step 5: Commit**

```bash
git add internal/status-updater/controllers/mock/controller.go \
        internal/status-updater/controllers/mock/controller_test.go
git commit -m "feat(mock): MockController — informer + handler (RUN-38195)"
```

---

## Task 20: Wire `MockController` in `app.go` (unconditional)

**Files:**
- Modify: `internal/status-updater/app.go`

The controller runs every time status-updater starts — when no `backend: mock` pools exist in the topology CM, it produces an empty desired set and does no work. No env-flag gate.

- [ ] **Step 1: Add the import**

Open `internal/status-updater/app.go`. In the import block, alongside other `controllers/<name>` imports, add:

```go
mockcontroller "github.com/run-ai/fake-gpu-operator/internal/status-updater/controllers/mock"
```

- [ ] **Step 2: Add the `EnvNvmlMockImage` constant**

Open `internal/common/constants/constants.go` and add:

```go
EnvNvmlMockImage = "NVML_MOCK_IMAGE"
```

Place it near the other `Env*` constants for grouping.

- [ ] **Step 3: Wire the controller unconditionally**

Find where other controllers are appended (around `app.Controllers = append(...)` lines for podcontroller, nodecontroller). Append after them, **before** any conditional controllers:

```go
pullPolicy := corev1.PullPolicy(viper.GetString("IMAGE_PULL_POLICY"))
if pullPolicy == "" {
    pullPolicy = corev1.PullAlways
}
app.Controllers = append(app.Controllers,
    mockcontroller.NewMockController(app.kubeClient, mockcontroller.ReconcileParams{
        Namespace:       viper.GetString(constants.EnvFakeGpuOperatorNs),
        Image:           viper.GetString(constants.EnvNvmlMockImage),
        ImagePullPolicy: pullPolicy,
    }))
```

If `corev1` isn't already imported in this file, add `corev1 "k8s.io/api/core/v1"` to the imports.

- [ ] **Step 4: Build and run all status-updater tests**

Run:
```bash
go vet ./internal/status-updater/...
go build ./internal/status-updater/...
go test ./internal/status-updater/... -count=1
```

Expected: clean vet, clean build, all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/status-updater/app.go internal/common/constants/constants.go
git commit -m "feat: wire MockController unconditionally with NVML_MOCK_IMAGE env (RUN-38195)"
```

---

## Task 21: Final verification

**Files:** *(none — verification only)*

- [ ] **Step 1: Full project vet + build**

Run:
```bash
go vet ./...
go build ./...
```

Expected: clean vet, clean build.

- [ ] **Step 2: Full unit-test suite**

Run: `go test ./internal/... -count=1`
Expected: all packages pass. (Compute-domain-dra-plugin tests may fail on macOS due to long-temp-path bind issues — those are environmental, unrelated to this PR. Verify the failures match upstream-expected by re-running on Linux or a CI runner.)

- [ ] **Step 3: Helm template render matrix**

Run:
```bash
helm dependency update deploy/fake-gpu-operator
test/helm/mock_backend_test.sh
```

Expected: `=== Summary: <N> passed, 0 failed ===` and exit 0.

- [ ] **Step 4: Helm lint**

Run: `helm lint deploy/fake-gpu-operator`
Expected: no errors. Warnings about subchart deprecations are acceptable as long as they're upstream-owned.

- [ ] **Step 5: Confirm chart default state matches polyfill expectations**

Run:
```bash
helm template fgo deploy/fake-gpu-operator | grep -E "name: gpu-operator$" | head -1
helm template fgo deploy/fake-gpu-operator | grep -E "MOCK_CONTROLLER_ENABLED" -A 1
```

Expected: first command finds the polyfill placeholder Deployment named `gpu-operator`; second shows `value: "false"`.

- [ ] **Step 6: Real-cluster runbook entry**

Append to `docs/superpowers/specs/2026-04-27-RUN-38195-mock-backend-design.md` — add a new section before "Out of test scope":

```markdown
### Real-cluster validation log

| Date | Cluster | Toggles | Outcome | Notes |
|---|---|---|---|---|
| YYYY-MM-DD | <user-provided cluster> | gpuOperator=true | PASS/FAIL | <link to logs> |
| YYYY-MM-DD | <user-provided cluster> | nvidiaDraDriver=true | PASS/FAIL | <link to logs> |
```

The first row is filled in by the implementer's manual test. Future bumps populate additional rows.

Commit the spec update:
```bash
git add docs/superpowers/specs/2026-04-27-RUN-38195-mock-backend-design.md
git commit -m "docs: add real-cluster validation log table (RUN-38195)"
```

- [ ] **Step 7: Confirm branch is clean and ready for PR**

Run:
```bash
git status
git log --oneline main..HEAD
```

Expected: clean working tree; commit log shows the sequence of commits from this plan's tasks plus the spec/plan docs.

---

## Task 22: Open PR with migration notes

**Files:** *(none — GitHub operations only)*

- [ ] **Step 1: Push the branch**

Run: `git push -u origin eliranw/RUN-38195-mock-backend`

- [ ] **Step 2: Create PR using `gh`**

Run:
```bash
gh pr create --title "feat: mock backend support (RUN-38195)" --body "$(cat <<'EOF'
## Summary

End-to-end Phase 5 mock-backend support. A `backend: mock` node pool now runs upstream NVIDIA GPU stack components (GPU Operator and/or DRA driver) against a mocked NVML driver layer (nvml-mock). Workloads requesting `nvidia.com/gpu` resources or DRA `ResourceClaim`s schedule on mock-pool nodes.

Three pieces ship together:
- **A** — new `internal/status-updater/controllers/mock/` package builds per-pool nvml-mock DaemonSets + ConfigMaps directly (no Helm SDK)
- **B** — two upstream Helm subcharts (`gpu-operator v26.3.1` and `nvidia-dra-driver-gpu v25.12.0`); existing GPU-Operator placeholder Deployment + ClusterPolicy CRD become a polyfill (gate inverts)
- **C** — `internal/status-updater/handlers/node/labels.go` narrows its predicate so `nvidia.com/gpu.deploy.*` labels apply only to nodes whose pool has `Gpu.Backend == "mock"`

Spec: `docs/superpowers/specs/2026-04-27-RUN-38195-mock-backend-design.md`

## Migration impact

| Change | Who's affected | Migration |
|---|---|---|
| `gpuOperator.enabled` default flips `true → false` | Default-config users | **No behavior change** — polyfill activates on the new default; matches what `enabled: true` did before |
| `gpuOperator.enabled=true` now installs the real subchart instead of the placeholder | Users explicitly setting `true` for the placeholder semantic | Set `false` to keep the placeholder, or accept the new behavior (requires real Linux GPU nodes + mock pools) |
| `gpuOperator.enabled=false` now activates the polyfill (was: nothing) | Users explicitly setting `false` to suppress both | Set `true` and let our subchart drive |
| `nvidia.com/gpu.deploy.*` labels narrowed to mock-pool nodes | Real-Linux-in-fake-pool deployments | Move those nodes into a `backend: mock` pool |

## Known incompatibility

`draPlugin.enabled: true` and `nvidiaDraDriver.enabled: true` both register a kubelet plugin for `nvidia.com/gpu` and conflict on overlapping nodes. Mutual exclusion documented; deprecating `draPlugin` is a follow-up.

## Test plan

- [ ] `go vet ./...` clean
- [ ] `go build ./...` clean
- [ ] `go test ./internal/...` clean
- [ ] `helm dependency update deploy/fake-gpu-operator` resolves both subcharts
- [ ] `test/helm/mock_backend_test.sh` passes
- [ ] `helm lint deploy/fake-gpu-operator` clean
- [ ] Real-cluster e2e (device-plugin path): `nvidia.com/gpu` workload runs on a mock-pool node
- [ ] Real-cluster e2e (DRA path, K8s 1.32+): DRA `ResourceClaim` workload runs on a mock-pool node
EOF
)"
```

- [ ] **Step 3: Note the PR URL**

Capture the URL printed by `gh pr create` for reference.

---

## Spec coverage check

| Spec section | Covered by |
|---|---|
| Architecture L1 (per-pool nvml-mock resources) | Tasks 13–20 |
| Architecture L2a (GPU Operator subchart) | Tasks 6, 7, 9, 12 |
| Architecture L2b (DRA driver subchart) | Tasks 6, 7, 12 |
| Architecture L3 (node labeling) | Handled by NFD chain (nvml-mock → gpu.present → gpu.deploy.*); see Tasks 4/5 deletion notes |
| Polyfill semantic | Tasks 7, 9, 12 |
| Mock controller wiring (unconditional) | Task 20 |
| nvml-mock SA | Task 8 |
| ClusterRole RBAC for daemonsets | Task 11 |
| `NvmlMock` in `ComponentsConfig` | Task 3 |
| Profile resolution + override merge | Task 14 |
| DaemonSet + ConfigMap builders | Task 15 |
| Diff strategy | Task 16 |
| Pure desired-state | Task 17 |
| Reconcile pipeline | Task 18 |
| Informer + handler | Task 19 |
| `app.go` wiring | Task 20 |
| Migration notes | Task 22 |
| Real-cluster runbook entry | Task 21 |

All spec requirements have a task. The `draPlugin` mutual-exclusion risk is noted in the PR description (Task 22) without code changes — matches the spec's "out of scope" deferral.
