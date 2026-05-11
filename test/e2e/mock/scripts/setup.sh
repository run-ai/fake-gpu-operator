#!/usr/bin/env bash

set -euo pipefail

SCRIPTS_DIR="$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd)"
PROJECT_ROOT="$(cd -- "${SCRIPTS_DIR}/../../../.." &> /dev/null && pwd)"
FIXTURES_DIR="$(cd -- "${SCRIPTS_DIR}/../fixtures" &> /dev/null && pwd)"

CURRENT_PLATFORM="linux/$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')"

: ${KIND_CLUSTER_NAME:="mock-cluster"}
: ${VALUES_FILE:="${FIXTURES_DIR}/values-mock.yaml"}
: ${KIND_CLUSTER_CONFIG_PATH:="${FIXTURES_DIR}/kind-cluster-config.yaml"}
: ${KIND_K8S_TAG:="v1.34.2"}
: ${KIND_IMAGE:="kindest/node:${KIND_K8S_TAG}"}
: ${DOCKER_TAG:="0.0.0-dev"}
: ${KWOK_VERSION:="v0.7.0"}

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

WORKER_NODES=("${KIND_CLUSTER_NAME}-worker" "${KIND_CLUSTER_NAME}-worker2")

echo "==> Installing nvidia-container-toolkit on each worker..."
# Each worker gets:
#  1. nvidia-container-toolkit (apt) — provides nvidia-container-runtime + nvidia-cdi-hook
#  2. containerd configured to use the runtime + CDI enabled
#  3. nvidia-container-runtime.mode = "cdi" — without this it defaults to "auto",
#     which falls back to legacy NVML enumeration on the host and fails with
#     ERROR_LIBRARY_NOT_FOUND because our mock libs aren't on the host's loader path
#  4. /run/nvidia/validations/toolkit-ready marker — gpu-operator's per-pod
#     toolkit-validation init container blocks on this file. With toolkit.enabled=false
#     the toolkit DaemonSet never creates it, so we fake it
#  5. /run/nvidia/driver pre-created as a symlink to /var/lib/nvml-mock/driver —
#     prevents a race where validator pods bind-mount it as an empty dir before
#     the nvml-mock DaemonSet starts and tries to lay down the symlink
for NODE in "${WORKER_NODES[@]}"; do
    echo "    -- $NODE"
    docker exec "$NODE" bash -c '
        set -e
        apt-get update -qq
        apt-get install -y -qq curl gpg
        curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey \
            | gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg
        curl -fsSL https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list \
            | sed "s#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g" \
            | tee /etc/apt/sources.list.d/nvidia-container-toolkit.list
        apt-get update -qq
        apt-get install -y -qq nvidia-container-toolkit
        nvidia-ctk runtime configure --runtime=containerd --cdi.enabled --set-as-default
        nvidia-ctk config --in-place --set nvidia-container-runtime.mode=cdi
        mkdir -p /run/nvidia/validations
        touch /run/nvidia/validations/toolkit-ready
        mkdir -p /var/lib/nvml-mock/driver
        rm -rf /run/nvidia/driver
        ln -sfn /var/lib/nvml-mock/driver /run/nvidia/driver
        # nvml-mock places its device nodes at $DRIVER_ROOT/dev (NVIDIA/k8s-test-infra
        # PR #326), where gpu-operator's driver-validator and the DRA driver's
        # getDevRoot() probe — so no host-side bare /dev/nvidia* nodes are needed.
        systemctl restart containerd
    '
done

echo "==> Waiting for nodes to settle after containerd restart..."
sleep 5
kubectl wait --for=condition=Ready nodes --all --timeout=120s

echo "==> Loading FGO images into kind..."
DOCKER_REPO_BASE="${DOCKER_REPO_BASE:-ghcr.io/run-ai/fake-gpu-operator}"
for component in dra-plugin-gpu status-updater status-exporter topology-server kwok-gpu-device-plugin kwok-dra-plugin kwok-compute-domain-dra-plugin compute-domain-controller compute-domain-dra-plugin; do
    IMAGE="${DOCKER_REPO_BASE}/${component}:${DOCKER_TAG}"
    echo "    -- ${IMAGE}"
    kind load docker-image --name "${KIND_CLUSTER_NAME}" "${IMAGE}"
done

# Note: cuda-sample image is not pre-loaded into kind. It's a manifest-list image
# (multi-arch), and `kind load` always runs `ctr import --all-platforms`, which
# fails on hosts that only have one arch locally. cuda-sample is publicly readable
# on NGC (no auth required), so kind nodes pull it at pod-startup time. The
# Ginkgo `podReadyTimeout` (3 min) absorbs this comfortably.

echo "==> Resolving subchart deps..."
cd "${PROJECT_ROOT}"
helm dependency update deploy/fake-gpu-operator

echo "==> Installing PrometheusRule CRD (used by status-updater for KWOK metrics)..."
kubectl apply -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/main/example/prometheus-operator-crd/monitoring.coreos.com_prometheusrules.yaml

echo "==> Installing fake-gpu-operator with mock e2e values..."
helm upgrade -i fake-gpu-operator deploy/fake-gpu-operator \
    --namespace gpu-operator \
    --create-namespace \
    -f "${VALUES_FILE}" \
    --set draPlugin.image.tag="${DOCKER_TAG}" \
    --set statusUpdater.image.tag="${DOCKER_TAG}" \
    --set statusExporter.image.tag="${DOCKER_TAG}" \
    --set topologyServer.image.tag="${DOCKER_TAG}" \
    --set kwokGpuDevicePlugin.image.tag="${DOCKER_TAG}" \
    --set kwokDraPlugin.image.tag="${DOCKER_TAG}" \
    --set computeDomainController.image.tag="${DOCKER_TAG}" \
    --set computeDomainDraPlugin.image.tag="${DOCKER_TAG}" \
    --set kwokComputeDomainDraPlugin.image.tag="${DOCKER_TAG}" \
    --wait --timeout 8m

echo "==> Waiting for status-updater pod ready..."
kubectl wait --for=condition=Ready pod -l app=status-updater -n gpu-operator --timeout=120s

echo "==> Waiting for nvml-mock DaemonSets ready..."
kubectl rollout status daemonset/nvml-mock-mock-a -n gpu-operator --timeout=180s
kubectl rollout status daemonset/nvml-mock-mock-b -n gpu-operator --timeout=180s

echo "==> Waiting for nvidia.com/gpu allocatable to populate on a mock-pool worker..."
# This is the real signal that the gpu-operator stack (NFD → operator → validator
# → device-plugin) finished bringing the node up. In gpu-operator v26.3.1 the
# 'gpu-operator-validator' Job from older versions was replaced by the
# 'nvidia-operator-validator' DaemonSet, and waiting on its Ready condition
# alone races the device-plugin's registration; allocatable is the canonical end-state.
DEADLINE=$(( $(date +%s) + 480 ))  # 8 min — covers worst-case operator reconcile + image pulls
while [ "$(date +%s)" -lt "$DEADLINE" ]; do
    GPU_QTY=$(kubectl get node "${WORKER_NODES[0]}" -o jsonpath='{.status.allocatable.nvidia\.com/gpu}' 2>/dev/null || true)
    if [ -n "$GPU_QTY" ] && [ "$GPU_QTY" != "0" ]; then
        echo "    -- ${WORKER_NODES[0]} reports nvidia.com/gpu=$GPU_QTY"
        break
    fi
    sleep 5
done
if [ "$(date +%s)" -ge "$DEADLINE" ]; then
    echo "WARN: nvidia.com/gpu allocatable did not populate. Dumping debug info..."
    kubectl get pods -n gpu-operator
    kubectl logs -l app=nvidia-operator-validator -n gpu-operator -c driver-validation --tail=50 || true
    exit 1
fi

echo "==> Waiting for nvidia-dra-driver-gpu kubelet plugin ready..."
# Cold-setup observation: this DaemonSet's first pod can take 3+ minutes to
# go Ready because the kubelet plugin probes the driver root via getDevRoot()
# at startup and may briefly read "/" before nvml-mock's bind mounts settle,
# then restart. 6 minutes covers that worst case.
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=nvidia-dra-driver-gpu -n gpu-operator --timeout=360s

# === Install KWOK + inject the fake-default fake node ===
echo "==> Installing KWOK controller..."
kubectl apply -f "https://github.com/kubernetes-sigs/kwok/releases/download/${KWOK_VERSION}/kwok.yaml"
kubectl wait --for=condition=Ready pod -l app=kwok-controller -n kube-system --timeout=120s
kubectl apply -f "https://github.com/kubernetes-sigs/kwok/releases/download/${KWOK_VERSION}/stage-fast.yaml"

echo "==> Injecting KWOK fake node kwok-fake-1 (pool: fake-default)..."
cat <<'YAML' | kubectl apply -f -
apiVersion: v1
kind: Node
metadata:
  annotations:
    kwok.x-k8s.io/node: fake
    node.alpha.kubernetes.io/ttl: "0"
  labels:
    kubernetes.io/role: worker
    node.kubernetes.io/instance-type: gpu-node
    run.ai/simulated-gpu-node-pool: fake-default
    type: kwok
  name: kwok-fake-1
spec:
  taints:
  - effect: NoSchedule
    key: kwok.x-k8s.io/node
    value: fake
status:
  allocatable:
    cpu: "32"
    memory: 128Gi
    pods: "110"
  capacity:
    cpu: "32"
    memory: 128Gi
    pods: "110"
  nodeInfo:
    architecture: amd64
    containerRuntimeVersion: fake
    kernelVersion: fake
    kubeProxyVersion: fake
    kubeletVersion: fake
    operatingSystem: linux
    osImage: fake
YAML

echo "==> Waiting for status-updater to create topology CM for kwok-fake-1..."
for i in {1..30}; do
    if kubectl get cm -n gpu-operator -l "node-name=kwok-fake-1" >/dev/null 2>&1; then
        echo "Topology CM created for kwok-fake-1!"
        break
    fi
    echo "Waiting... ($i/30)"
    sleep 2
done

echo "==> Waiting for ResourceSlice for kwok-fake-1..."
for i in {1..30}; do
    if kubectl get resourceslice "kwok-kwok-fake-1-gpu" >/dev/null 2>&1; then
        echo "ResourceSlice created for kwok-fake-1!"
        break
    fi
    echo "Waiting... ($i/30)"
    sleep 2
done

echo ""
echo "===================================================="
echo "Setup complete! Cluster ${KIND_CLUSTER_NAME} is ready."
echo "  Real workers: ${WORKER_NODES[*]}"
echo "  KWOK fake node: kwok-fake-1 (pool: fake-default)"
echo "  Pools: mock-a (worker-1, a100) | mock-b (worker-2, h100) | fake-default (kwok-fake-1, a100 fake)"
echo "===================================================="
