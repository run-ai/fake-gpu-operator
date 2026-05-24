# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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
  predate that key. Wired into CI as a release gate. (RUN-39195)

### Changed

- `gpu-operator` subchart defaults adjusted for mock-pool usage:
  `gpu-operator.gfd.enabled` now defaults to `false` (FGO's status-exporter
  already writes the labels GFD would; GFD's pod can't load mock NVML
  anyway), and `gpu-operator.toolkit.env` now defaults to
  `[CREATE_DEVICE_NODES=none]` so the toolkit installer skips real-device
  enumeration when users flip `toolkit.enabled: true`. (RUN-38195)

### Fixed

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
