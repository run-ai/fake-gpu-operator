// Package mock implements the status-updater MockController. It watches
// the cluster topology ConfigMap and reconciles per-pool nvml-mock
// DaemonSets and ConfigMaps for pools whose backend is "mock".
//
// The controller runs unconditionally inside status-updater. When no
// backend:mock pools exist, it produces an empty desired set and does
// no work — no env-flag gate.
//
// This package does NOT use the Helm SDK at runtime — per-pool resources
// are built directly as Kubernetes objects, mirroring the shape of
// upstream nvml-mock's templates/daemonset.yaml.
//
// Spec: docs/superpowers/specs/2026-04-27-RUN-38195-mock-backend-design.md
package mock
