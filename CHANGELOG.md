# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- README: "Mixed Real + Fake GPU Nodes" guide for running alongside a real NVIDIA GPU Operator (install in a separate namespace and disable the colliding `devicePlugin`, `statusExporter`, and `runtimeClass` components).

### Changed

- Built-in GPU profiles re-synced from NVIDIA/k8s-test-infra `main` (commit
  `497fa04`): each profile now includes a `pcie_topology` block (PCI root
  complexes with per-device `numa_node`), and a `gb300` profile is added. This
  is what lets the mock backend report per-GPU NUMA affinity. (RUN-40173)
- `hack/sync-profiles.sh`: default source bumped `v0.1.0` → `main`; now resolves
  a tag, branch, or commit SHA (was tags/branches only) and records the resolved
  commit in the generated `# Source:` header. (RUN-40173)

### Fixed

- `device-plugin` injects `NODE_NAME` so non-DRA pods can run the fake `nvidia-smi`. ([#191](https://github.com/run-ai/fake-gpu-operator/issues/191))
- `sync-gpu-profiles` workflow read the synced version with `head -2`, but the `# Source:` line is line 3, so the PR title/commit version was always empty. (RUN-40173)
- CI `e2e-upgrade (latest-main)` lane no longer deadlocks resolving its baseline
  chart. It now walks `--first-parent` main commits (only those publish a
  `0.0.0-<sha>` chart, so merges no longer fill the window with unpublished
  feature-branch commits), widens the lookback, and falls back to the latest
  release tag instead of hard-failing — a hard failure here skipped
  `release-helm`, which then never published the chart the next run needed.
  Mirrored in the `make e2e-upgrade-from-main` target. (RUN-40080)
- Fake `nvidia-smi` exits gracefully instead of panicking on errors. ([#206](https://github.com/run-ai/fake-gpu-operator/issues/206))
- Fake `nvidia-smi` failure output mirrors real `nvidia-smi` per error instead of one generic line.


## [0.0.81] - 2026-05-27

### Added

- `CONTRIBUTING.md`, `.github/PULL_REQUEST_TEMPLATE.md`, and structured issue
  templates (`BUG_REPORT.yaml`, `ENHANCEMENT.yaml`) to guide external
  contributors. ([RUN-38925](https://runai.atlassian.net/browse/RUN-38925))
- `CHANGELOG.md` following [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
  format. PRs should add an entry here under `## [Unreleased]` unless the
  `skip-changelog` label is applied.
- Mock backend for per-pool `nvml-mock` integration: nodes in pools with
  `gpu.backend: mock` get a profile-driven `nvml-mock` DaemonSet plus
  ConfigMap, exposing a real `libnvidia-ml.so` so the upstream NVIDIA
  device-plugin and DRA driver enumerate synthetic GPUs through NVML.
  Toggles via new `gpuOperator.enabled` / `nvidiaDraDriver.enabled` chart
  subcharts. See `docs/mock-backend.md`.
  ([RUN-38195](https://runai.atlassian.net/browse/RUN-38195))
- KIND-based mock-pool e2e suite (`make e2e-mock`) covering the
  device-plugin path, DRA path, multi-pool differentiation, profile
  overrides, and fake/mock coexistence. Wired into CI as a release gate.
  ([RUN-38195](https://runai.atlassian.net/browse/RUN-38195))
- Helm-upgrade e2e suite (`make e2e-upgrade`) that installs a pinned
  published OCI baseline chart and then upgrades to the chart on the
  current branch with the same values. Catches the regression class
  where a new top-level chart value gets referenced unsafely in a
  template and breaks `helm upgrade` for users whose stored values
  predate that key. In CI runs as a matrix with two baselines — the
  pinned release and the latest published `main` chart (via
  `make e2e-upgrade-from-main` locally) — so regressions are caught
  both against shipped releases and against not-yet-released main.
  The suite is split into three idempotent stages for ad-hoc iteration:
  `make setup-e2e-upgrade` (cluster + baseline), `make upgrade-e2e-upgrade`
  (apply HEAD chart, re-runnable after each chart edit), and
  `make test-e2e-upgrade` (assertions only). Wired into CI as a release
  gate. (RUN-39195)

### Changed

- `gpu-operator` subchart defaults adjusted for mock-pool usage:
  `gpu-operator.gfd.enabled` now defaults to `false` (FGO's status-exporter
  already writes the labels GFD would; GFD's pod can't load mock NVML
  anyway), and `gpu-operator.toolkit.env` now defaults to
  `[CREATE_DEVICE_NODES=none]` so the toolkit installer skips real-device
  enumeration when users flip `toolkit.enabled: true`. (RUN-38195)

### Fixed

- `kwok-dra-plugin` ResourceSlices failing upstream `nvidia-dra-driver-gpu`
  DeviceClass CEL selector on hybrid clusters. Pods requesting
  `gpu.nvidia.com` ResourceClaims and targeting KWOK fake nodes hit
  `CEL runtime error: no such key: type` because slices only carried
  unqualified `uuid`/`model` attributes — upstream's selector reads
  `device.attributes['gpu.nvidia.com'].type`. The plugin now also emits
  qualified `gpu.nvidia.com/type=gpu`, `gpu.nvidia.com/uuid`, and
  `gpu.nvidia.com/productName`, and the chart's own `gpu.nvidia.com`
  DeviceClass mirrors upstream's CEL + `extendedResourceName` so the
  contract is identical regardless of whether `nvidiaDraDriver.enabled`
  is set. (RUN-39005)
- `status-updater` mock controller emitting constant `configmaps "topology"
  is forbidden: cannot watch` errors. The chart's `fake-status-updater`
  ClusterRole was missing the `watch` verb on `configmaps`, so the informer
  added in Phase 5 (mock backend) could never establish a watch and fell
  back to polling-style reconciles.
  ([RUN-38195](https://runai.atlassian.net/browse/RUN-38195))
- Mock-pool operand DaemonSets (`nvidia-device-plugin-daemonset`,
  `gpu-feature-discovery`, `nvidia-operator-validator`) blocking at
  `Init:0/1` forever on mock-NVML nodes. Their hardcoded `toolkit-validation`
  init container polls for `/run/nvidia/validations/toolkit-ready`, but
  nothing wrote that marker on mock setups — the upstream gpu-operator
  validator that normally writes it can't `exec nvidia-smi` in its
  isolated init container. Result: `nvidia.com/gpu` never advertised, no
  workload schedulable. The per-pool `nvml-mock` DaemonSet now backgrounds
  upstream's `entrypoint.sh` and writes the marker once `setup.sh`
  signals completion (the `/run/nvidia/driver` symlink). Upstream's
  script is preserved verbatim so future setup-script evolution flows
  through automatically. Interim until [NVIDIA/k8s-test-infra#346](https://github.com/NVIDIA/k8s-test-infra/pull/346)
  lands the marker write in nvml-mock's `setup.sh` upstream.
