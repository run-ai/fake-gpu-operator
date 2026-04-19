#!/usr/bin/env bash

set -e

# Orchestrates integration tests for both values formats:
#   1. Run tests with old format (topology: key) — already deployed by setup.sh
#   2. Helm upgrade to profile-based format (cluster: key with GPU profiles)
#   3. Run tests again with profile-based expected values

SCRIPTS_DIR="$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd)"
PROJECT_ROOT="$(cd -- "${SCRIPTS_DIR}/../.." &> /dev/null && pwd)"

: ${DOCKER_TAG:="0.0.0-dev"}
: ${GINKGO:="${PROJECT_ROOT}/bin/ginkgo"}

# ─────────────────────────────────────────────────
# Phase 1: Old format (topology: key)
# ─────────────────────────────────────────────────
echo ""
echo "════════════════════════════════════════════════"
echo "  Phase 1: Integration tests — old format"
echo "════════════════════════════════════════════════"
echo ""

cd "${SCRIPTS_DIR}"
EXPECTED_GPU_PRODUCT="NVIDIA-A100-SXM4-40GB" \
EXPECTED_GPU_COUNT="2" \
EXPECTED_HIGHEND_GPU_PRODUCT="NVIDIA-H100-80GB-HBM3" \
EXPECTED_HIGHEND_GPU_COUNT="4" \
    "${GINKGO}" --procs=1 --timeout=30m --trace

# ─────────────────────────────────────────────────
# Upgrade: switch to profile-based config
# ─────────────────────────────────────────────────
echo ""
echo "════════════════════════════════════════════════"
echo "  Upgrading Helm release to profile-based config"
echo "════════════════════════════════════════════════"
echo ""

cd "${PROJECT_ROOT}"
helm upgrade fake-gpu-operator deploy/fake-gpu-operator \
    --namespace gpu-operator \
    -f "${SCRIPTS_DIR}/values-profiles.yaml" \
    --set draPlugin.image.tag="${DOCKER_TAG}" \
    --set statusUpdater.image.tag="${DOCKER_TAG}" \
    --set statusExporter.image.tag="${DOCKER_TAG}" \
    --set topologyServer.image.tag="${DOCKER_TAG}" \
    --set kwokDraPlugin.image.tag="${DOCKER_TAG}" \
    --set computeDomainController.image.tag="${DOCKER_TAG}" \
    --set computeDomainDraPlugin.image.tag="${DOCKER_TAG}" \
    --set kwokComputeDomainDraPlugin.image.tag="${DOCKER_TAG}"

# Delete per-node topology CMs and ResourceSlices so the status-updater
# recreates them from the new config. The status-updater only creates CMs —
# it won't update existing ones.
KWOK_NODES=("kwok-gpu-node-1" "kwok-gpu-node-2" "kwok-gpu-node-3" "kwok-gpu-node-4" "kwok-gpu-node-5")
echo "Deleting existing per-node topology ConfigMaps and ResourceSlices..."
for NODE_NAME in "${KWOK_NODES[@]}"; do
    kubectl delete cm -n gpu-operator -l "node-name=${NODE_NAME}" --ignore-not-found=true
    kubectl delete resourceslice "kwok-${NODE_NAME}-gpu" --ignore-not-found=true
done

echo "Waiting for status-updater to restart..."
kubectl rollout status deployment/status-updater -n gpu-operator --timeout=120s

echo "Waiting for status-exporter to restart..."
kubectl rollout status daemonset/nvidia-dcgm-exporter -n gpu-operator --timeout=120s

echo "Waiting for kwok-dra-plugin to restart..."
kubectl rollout status deployment/kwok-dra-plugin -n gpu-operator --timeout=120s

# Wait for topology CMs to be recreated with profile-resolved values.
echo "Waiting for topology ConfigMaps to be recreated..."
for NODE_NAME in "${KWOK_NODES[@]}"; do
    for i in {1..30}; do
        if kubectl get cm -n gpu-operator -l "node-name=${NODE_NAME}" -o jsonpath='{.items[0].data.topology\.yml}' 2>/dev/null | grep -q "gpuProduct"; then
            echo "Topology ConfigMap recreated for ${NODE_NAME}"
            break
        fi
        if [[ $i -eq 30 ]]; then
            echo "ERROR: Timed out waiting for topology ConfigMap for ${NODE_NAME}"
            exit 1
        fi
        echo "Waiting for topology ConfigMap... ($i/30)"
        sleep 2
    done
done

# Wait for ResourceSlices to be recreated by kwok-dra-plugin
for NODE_NAME in "${KWOK_NODES[@]}"; do
    for i in {1..30}; do
        if kubectl get resourceslice "kwok-${NODE_NAME}-gpu" >/dev/null 2>&1; then
            echo "ResourceSlice recreated for ${NODE_NAME}"
            break
        fi
        if [[ $i -eq 30 ]]; then
            echo "ERROR: Timed out waiting for ResourceSlice for ${NODE_NAME}"
            exit 1
        fi
        echo "Waiting for ResourceSlice... ($i/30)"
        sleep 2
    done
done

# Give status-exporter time to update node labels from new topology
echo "Waiting for node labels to converge..."
sleep 5

# ─────────────────────────────────────────────────
# Phase 2: Profile-based format (cluster: key)
# ─────────────────────────────────────────────────
echo ""
echo "════════════════════════════════════════════════"
echo "  Phase 2: Integration tests — profile-based"
echo "════════════════════════════════════════════════"
echo ""

cd "${SCRIPTS_DIR}"
EXPECTED_GPU_PRODUCT="NVIDIA T4" \
EXPECTED_GPU_COUNT="2" \
EXPECTED_HIGHEND_GPU_PRODUCT="NVIDIA H100 80GB HBM3" \
EXPECTED_HIGHEND_GPU_COUNT="4" \
    "${GINKGO}" --procs=1 --timeout=30m --trace

echo ""
echo "════════════════════════════════════════════════"
echo "  All integration tests passed!"
echo "════════════════════════════════════════════════"
