#!/usr/bin/env bash

set -euo pipefail

# A reference to the directory where this script is located
SCRIPTS_DIR="$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd)"
PROJECT_ROOT="$(cd -- "${SCRIPTS_DIR}/../../../.." &> /dev/null && pwd)"
FIXTURES_DIR="$(cd -- "${SCRIPTS_DIR}/../fixtures" &> /dev/null && pwd)"

# --load only works with single-platform builds, so detect the current platform
CURRENT_PLATFORM="linux/$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')"

: ${KIND_CLUSTER_NAME:="device-plugin-cluster"}
: ${VALUES_FILE:="${FIXTURES_DIR}/values.yaml"}
: ${KIND_CLUSTER_CONFIG_PATH:="${FIXTURES_DIR}/kind-cluster-config.yaml"}
: ${KIND_K8S_TAG:="v1.34.0"}
: ${KIND_IMAGE:="kindest/node:${KIND_K8S_TAG}"}
: ${DOCKER_TAG:="0.0.0-dev"}

# The worker node dedicated to the device-plugin / MIG path. kind names workers
# "<cluster>-worker".
WORKER_NODE="${KIND_CLUSTER_NAME}-worker"
MIG_POOL="mig-pool"

if ! command -v docker &> /dev/null; then
    echo "Docker not found. Please install Docker."
    exit 1
fi
echo "Docker found in PATH."

if [[ "${SKIP_SETUP:-false}" == "true" ]]; then
    echo "Skipping setup (SKIP_SETUP=true)"
    exit 0
fi

if kind get clusters | grep -q "^${KIND_CLUSTER_NAME}$"; then
    echo "Cluster ${KIND_CLUSTER_NAME} already exists. Use SKIP_SETUP=true or delete the cluster first."
    exit 1
fi

echo "==> Building Docker images (platform ${CURRENT_PLATFORM})..."
cd "${PROJECT_ROOT}"
make image DOCKER_TAG="${DOCKER_TAG}" DOCKER_BUILDX_PLATFORMS="${CURRENT_PLATFORM}" DOCKER_BUILDX_PUSH_FLAG="--load"

echo "==> Creating kind cluster ${KIND_CLUSTER_NAME}..."
kind create cluster \
    --name "${KIND_CLUSTER_NAME}" \
    --image "${KIND_IMAGE}" \
    --config "${KIND_CLUSTER_CONFIG_PATH}" \
    --wait 2m

echo "==> Loading FGO images into kind..."
DOCKER_REPO_BASE="${DOCKER_REPO_BASE:-ghcr.io/run-ai/fake-gpu-operator}"
for component in device-plugin mig-faker status-updater topology-server; do
    IMAGE="${DOCKER_REPO_BASE}/${component}:${DOCKER_TAG}"
    echo "    -- ${IMAGE}"
    kind load docker-image --name "${KIND_CLUSTER_NAME}" "${IMAGE}"
done

echo "==> Waiting for nodes to be ready..."
kubectl wait --for=condition=Ready nodes --all --timeout=120s

# Label the worker for the device-plugin + mig-faker DaemonSets. The pool label
# is already applied via kind config; these are the MIG-specific selectors that
# would normally come from NFD/GFD on a real GPU node:
#   - nvidia.com/gpu.present                  -> mig-faker nodeAffinity
#   - node-role.kubernetes.io/runai-dynamic-mig -> mig-faker nodeSelector
#   - nvidia.com/gpu.deploy.device-plugin     -> device-plugin nodeSelector
#     (status-updater also sets this, but set it now to avoid startup crashloops)
echo "==> Labelling worker ${WORKER_NODE} for the device-plugin / MIG path..."
kubectl label node "${WORKER_NODE}" \
    nvidia.com/gpu.present=true \
    node-role.kubernetes.io/runai-dynamic-mig=true \
    nvidia.com/gpu.deploy.device-plugin=true \
    --overwrite

echo "==> Resolving subchart deps..."
cd "${PROJECT_ROOT}"
helm dependency update deploy/fake-gpu-operator

echo "==> Installing fake-gpu-operator (device-plugin + mig-faker)..."
helm upgrade -i fake-gpu-operator deploy/fake-gpu-operator \
    --namespace gpu-operator \
    --create-namespace \
    -f "${VALUES_FILE}" \
    --set devicePlugin.image.tag="${DOCKER_TAG}" \
    --set migFaker.image.tag="${DOCKER_TAG}" \
    --set statusUpdater.image.tag="${DOCKER_TAG}" \
    --set topologyServer.image.tag="${DOCKER_TAG}" \
    --wait --timeout 5m

echo "==> Waiting for status-updater pod ready..."
kubectl wait --for=condition=Ready pod -l app=status-updater -n gpu-operator --timeout=120s

echo "==> Waiting for topology-server pod ready..."
kubectl wait --for=condition=Ready pod -l app=topology-server -n gpu-operator --timeout=120s

echo "==> Waiting for status-updater to create the topology ConfigMap for ${WORKER_NODE}..."
for i in {1..30}; do
    if kubectl get cm -n gpu-operator -l "node-name=${WORKER_NODE}" >/dev/null 2>&1 && \
       [ -n "$(kubectl get cm -n gpu-operator -l "node-name=${WORKER_NODE}" -o name)" ]; then
        echo "Topology ConfigMap created for ${WORKER_NODE}!"
        break
    fi
    echo "Waiting for topology ConfigMap... ($i/30)"
    sleep 2
done

echo "==> Waiting for device-plugin DaemonSet pod ready..."
kubectl rollout status daemonset/device-plugin -n gpu-operator --timeout=180s

echo "==> Waiting for mig-faker DaemonSet pod ready..."
kubectl rollout status daemonset/mig-faker -n gpu-operator --timeout=180s

echo "==> Waiting for nvidia.com/gpu allocatable to populate on ${WORKER_NODE}..."
DEADLINE=$(( $(date +%s) + 180 ))
while [ "$(date +%s)" -lt "$DEADLINE" ]; do
    GPU_QTY=$(kubectl get node "${WORKER_NODE}" -o jsonpath='{.status.allocatable.nvidia\.com/gpu}' 2>/dev/null || true)
    if [ -n "${GPU_QTY}" ] && [ "${GPU_QTY}" != "0" ]; then
        echo "    -- ${WORKER_NODE} reports nvidia.com/gpu=${GPU_QTY}"
        break
    fi
    sleep 3
done

echo ""
echo "===================================================="
echo "Setup complete! Cluster ${KIND_CLUSTER_NAME} is ready."
echo "  MIG worker: ${WORKER_NODE} (pool: ${MIG_POOL})"
echo "===================================================="
