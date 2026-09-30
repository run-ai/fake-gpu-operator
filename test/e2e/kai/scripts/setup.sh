#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd -- "${SCRIPT_DIR}/../../../.." && pwd)"

export KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-fake-gpu-operator-kai}"
DOCKER_TAG="${DOCKER_TAG:-0.0.0-dev}"
DOCKER_REPO_BASE="${DOCKER_REPO_BASE:-ghcr.io/run-ai/fake-gpu-operator}"
KIND_IMAGE="${KIND_IMAGE:-kindest/node:v1.34.0}"
CURRENT_PLATFORM="linux/$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')"
COMPONENTS=(device-plugin status-updater status-exporter topology-server)

if kind get clusters | grep -qx "${KIND_CLUSTER_NAME}"; then
    echo "Cluster ${KIND_CLUSTER_NAME} already exists"
    exit 1
fi

make -C "${PROJECT_ROOT}" image \
    COMPONENTS="${COMPONENTS[*]}" \
    DOCKER_REPO_BASE="${DOCKER_REPO_BASE}" \
    DOCKER_TAG="${DOCKER_TAG}" \
    DOCKER_BUILDX_PLATFORMS="${CURRENT_PLATFORM}" \
    DOCKER_BUILDX_PUSH_FLAG=--load

kind create cluster \
    --name "${KIND_CLUSTER_NAME}" \
    --image "${KIND_IMAGE}" \
    --config "${PROJECT_ROOT}/test/e2e/fixtures/kind-cluster-config.yaml" \
    --wait 2m

for component in "${COMPONENTS[@]}"; do
    kind load docker-image --name "${KIND_CLUSTER_NAME}" \
        "${DOCKER_REPO_BASE}/${component}:${DOCKER_TAG}"
done

kubectl wait --for=condition=Ready nodes --all --timeout=120s

helm dependency update "${PROJECT_ROOT}/deploy/fake-gpu-operator"
helm upgrade -i fake-gpu-operator "${PROJECT_ROOT}/deploy/fake-gpu-operator" \
    --namespace gpu-operator --create-namespace \
    -f "${PROJECT_ROOT}/test/e2e/kai/fixtures/values.yaml" \
    --set devicePlugin.image.tag="${DOCKER_TAG}" \
    --set statusUpdater.image.tag="${DOCKER_TAG}" \
    --set statusExporter.image.tag="${DOCKER_TAG}" \
    --set topologyServer.image.tag="${DOCKER_TAG}"

kubectl -n gpu-operator wait --for=condition=Available deployment/status-updater --timeout=120s
kubectl -n gpu-operator rollout status daemonset/device-plugin --timeout=180s
kubectl -n gpu-operator rollout status daemonset/nvidia-dcgm-exporter --timeout=180s
kubectl -n gpu-operator wait --for=condition=Available deployment/topology-server --timeout=120s

if [[ -z "${KAI_VERSION:-}" ]]; then
    KAI_VERSION="$(curl -fsSL --retry 3 https://api.github.com/repos/kai-scheduler/KAI-Scheduler/releases/latest | jq -er '.tag_name')"
fi
echo "Installing KAI Scheduler ${KAI_VERSION}"

HELM_ARGS=(
    upgrade -i kai-scheduler oci://ghcr.io/kai-scheduler/kai-scheduler/kai-scheduler
    --namespace kai-scheduler --create-namespace
    --version "${KAI_VERSION}"
    --set global.gpuSharingMode=NonMemoryEnforced
    --set-string admission.gpuFractionRuntimeClassName=
    --wait --timeout 10m
)
helm "${HELM_ARGS[@]}"

kubectl -n kai-scheduler wait --for=condition=Available deployment --all --timeout=300s
kubectl -n kai-scheduler get pods
