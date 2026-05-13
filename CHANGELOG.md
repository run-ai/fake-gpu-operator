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

### Changed

### Fixed

- `status-updater` mock controller emitting constant `configmaps "topology"
  is forbidden: cannot watch` errors. The chart's `fake-status-updater`
  ClusterRole was missing the `watch` verb on `configmaps`, so the informer
  added in Phase 5 (mock backend) could never establish a watch and fell
  back to polling-style reconciles.
  ([RUN-38195](https://runai.atlassian.net/browse/RUN-38195))
