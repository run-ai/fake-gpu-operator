#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd -- "${SCRIPT_DIR}/../../../.." && pwd)"

export KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-fake-gpu-operator-kai}"
export RESOURCE_RESERVATION_NAMESPACE=kai-resource-reservation

"${PROJECT_ROOT}/test/e2e/scripts/setup.sh"

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
