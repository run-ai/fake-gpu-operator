#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd -- "${SCRIPT_DIR}/../../../.." && pwd)"

export KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-fake-gpu-operator-kai}"
export RESOURCE_RESERVATION_NAMESPACE=kai-resource-reservation

"${PROJECT_ROOT}/test/e2e/scripts/setup.sh"

HELM_ARGS=(
    upgrade -i kai-scheduler oci://ghcr.io/kai-scheduler/kai-scheduler/kai-scheduler
    --namespace kai-scheduler --create-namespace
    --set global.gpuSharingMode=NonMemoryEnforced
    --set-string admission.gpuFractionRuntimeClassName=
    --wait --timeout 10m
)
if [[ -n "${KAI_VERSION:-}" ]]; then
    HELM_ARGS+=(--version "${KAI_VERSION}")
fi
helm "${HELM_ARGS[@]}"

kubectl -n kai-scheduler wait --for=condition=Available deployment --all --timeout=300s
kubectl -n kai-scheduler get pods
