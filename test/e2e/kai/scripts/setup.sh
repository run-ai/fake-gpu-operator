#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd -- "${SCRIPT_DIR}/../../../.." && pwd)"

export KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-fake-gpu-operator-kai}"
export RESOURCE_RESERVATION_NAMESPACE=kai-resource-reservation

"${PROJECT_ROOT}/test/e2e/scripts/setup.sh"

KAI_VERSION="${KAI_VERSION:-v0.17.0}"
helm upgrade -i kai-scheduler oci://ghcr.io/kai-scheduler/kai-scheduler/kai-scheduler \
    --namespace kai-scheduler --create-namespace \
    --version "${KAI_VERSION}" \
    --set global.gpuSharing=true \
    --set-string admission.gpuFractionRuntimeClassName= \
    --wait --timeout 10m

kubectl -n kai-scheduler wait --for=condition=Available deployment --all --timeout=300s
kubectl -n kai-scheduler get pods
