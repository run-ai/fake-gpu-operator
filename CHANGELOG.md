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

- `gpu-operator` subchart defaults adjusted for mock-pool usage:
  `gpu-operator.gfd.enabled` now defaults to `false` (FGO's status-exporter
  already writes the labels GFD would; GFD's pod can't load mock NVML
  anyway), and `gpu-operator.toolkit.env` now defaults to
  `[CREATE_DEVICE_NODES=none]` so the toolkit installer skips real-device
  enumeration when users flip `toolkit.enabled: true`. (RUN-38195)
- `gpu-operator.dcgm.enabled` and `gpu-operator.dcgmExporter.enabled` now
  default to `true`. When `gpuOperator.enabled: true`, the upstream
  state-dcgm-exporter reconciler creates the `nvidia-dcgm-exporter`
  Service that `runai-cluster`'s GPU-stack health check polls — without
  this, runaiconfig stayed `Reconciled=False` with
  `dcgm-exporter service not found in the cluster`. The exporter pod
  itself still crashloops on mock-NVML (no DCGM bindings in nvml-mock),
  but the Service existence is what runaiconfig validates. (RUN-38195)
- Polyfill `Deployment/gpu-operator` (rendered when `gpuOperator.enabled:
  false`) now uses the same selector labels as the upstream gpu-operator
  subchart (`app.kubernetes.io/component: gpu-operator` +
  `app: gpu-operator`). Without this, upgrading from
  `gpuOperator.enabled: false -> true` failed with
  `Deployment.apps "gpu-operator" is invalid: spec.selector: field is
  immutable` because helm tried to patch the polyfill into the
  subchart-shaped Deployment. (RUN-38195)

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
  signals full completion (writing `/run/nvidia/validations/driver-ready`,
  its terminal artifact). The wrapper previously gated on the
  `/run/nvidia/driver` symlink — an *earlier* step that `setup.sh`
  followed by a `rm -rf /run/nvidia/validations` recreate, wiping the
  marker. Gating on `driver-ready` (the last file written) is race-free.
  Upstream's script is preserved verbatim so future setup-script evolution
  flows through automatically. Interim until [NVIDIA/k8s-test-infra#346](https://github.com/NVIDIA/k8s-test-infra/pull/346)
  lands the marker write in nvml-mock's `setup.sh` upstream.
- Polyfill `RuntimeClass/nvidia` (rendered when `gpuOperator.enabled:
  false`) used `handler: runc`, while the upstream gpu-operator subchart
  creates the same RuntimeClass with `handler: nvidia`. RuntimeClass
  `.handler` is immutable, so upgrading from
  `gpuOperator.enabled: false -> true` failed with
  `RuntimeClass.node.k8s.io "nvidia" is invalid: handler: Invalid value:
  "nvidia": field is immutable`. Fix: gate the polyfill on
  `(not gpuOperator.enabled)`, and add a `pre-upgrade` hook Job that
  deletes the stale `handler: runc` RuntimeClass during the transition
  so the upstream subchart can recreate it with the correct handler.
  (RUN-38195)
- Conflicting `nvidia-dcgm-exporter` Service when both
  `statusExporter.enabled: true` and `gpuOperator.enabled: true`. FGO's
  status-exporter and the upstream gpu-operator's state-dcgm-exporter
  reconciler both produce a Service of that name in the chart's
  namespace; upstream wins (its reconciler deletes anything it doesn't
  own) and FGO's resources get garbage-collected within seconds. Added a
  chart-level `{{ fail }}` validation in `templates/_validation.tpl` that
  aborts `helm install`/`helm upgrade` at render time with guidance on
  which toggle to flip. (RUN-38195)
