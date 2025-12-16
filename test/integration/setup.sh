#!/usr/bin/env bash

set -e

# A reference to the current directory where this script is located
SCRIPTS_DIR="$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd)"
PROJECT_ROOT="$(cd -- "${SCRIPTS_DIR}/../.." &> /dev/null && pwd)"

# For integration tests, we need to load images into docker (not push to registry)
# --load only works with single-platform builds, so we detect the current platform
CURRENT_PLATFORM="linux/$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')"
# The name of the kind cluster to create
: ${KIND_CLUSTER_NAME:="fake-gpu-operator-cluster"}

# The path to kind's cluster configuration file
: ${KIND_CLUSTER_CONFIG_PATH:="${SCRIPTS_DIR}/kind-cluster-config.yaml"}

# Kubernetes version for kind
: ${KIND_K8S_TAG:="v1.34.0"}

# The name of the kind image to use
: ${KIND_IMAGE:="kindest/node:${KIND_K8S_TAG}"}

# Docker image tag
: ${DOCKER_TAG:="0.0.0-dev"}

# Container tool, e.g. docker/podman
if [[ -z "${CONTAINER_TOOL}" ]]; then
    if [[ -n "$(which docker)" ]]; then
        echo "Docker found in PATH."
        CONTAINER_TOOL=docker
    elif [[ -n "$(which podman)" ]]; then
        echo "Podman found in PATH."
        CONTAINER_TOOL=podman
    else
        echo "No container tool detected. Please install Docker or Podman."
        exit 1
    fi
fi

: ${KIND:="env KIND_EXPERIMENTAL_PROVIDER=${CONTAINER_TOOL} kind"}

# Check if cluster already exists
if [[ "${SKIP_SETUP}" != "true" ]]; then
    if ${KIND} get clusters | grep -q "^${KIND_CLUSTER_NAME}$"; then
        echo "Cluster ${KIND_CLUSTER_NAME} already exists. Use SKIP_SETUP=true to skip setup or delete the cluster first."
        exit 1
    fi

    echo "Building Docker images for platform ${CURRENT_PLATFORM}..."
    cd "${PROJECT_ROOT}"
    # Use --load to load images into docker, with single platform (required for --load)
    make image DOCKER_TAG="${DOCKER_TAG}" DOCKER_BUILDX_PLATFORMS="${CURRENT_PLATFORM}" DOCKER_BUILDX_PUSH_FLAG="--load"

    echo "Creating kind cluster ${KIND_CLUSTER_NAME}..."
    ${KIND} create cluster \
        --name "${KIND_CLUSTER_NAME}" \
        --image "${KIND_IMAGE}" \
        --config "${KIND_CLUSTER_CONFIG_PATH}" \
        --wait 2m

    echo "Loading images into kind cluster..."
    DOCKER_REPO_BASE="${DOCKER_REPO_BASE:-ghcr.io/run-ai/fake-gpu-operator}"
    for component in dra-plugin-gpu status-updater topology-server kwok-dra-plugin; do
        IMAGE="${DOCKER_REPO_BASE}/${component}:${DOCKER_TAG}"
        echo "Loading ${IMAGE}..."
        if [[ "${CONTAINER_TOOL}" == "podman" ]]; then
            # Work around kind not loading image with podman
            IMAGE_ARCHIVE="/tmp/${component}_image.tar"
            ${CONTAINER_TOOL} save -o "${IMAGE_ARCHIVE}" "${IMAGE}" && \
            ${KIND} load image-archive \
                --name "${KIND_CLUSTER_NAME}" \
                "${IMAGE_ARCHIVE}"
            rm -f "${IMAGE_ARCHIVE}"
        else
            ${KIND} load docker-image \
                --name "${KIND_CLUSTER_NAME}" \
                "${IMAGE}"
        fi
    done

    echo "Waiting for nodes to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=120s

    echo "Labeling all nodes for fake GPU operator..."
    # Get all node names
    NODES=$(kubectl get nodes -o jsonpath='{.items[*].metadata.name}')
    for NODE in ${NODES}; do
        echo "Labeling node ${NODE}..."
        kubectl label node "${NODE}" nvidia.com/gpu.deploy.dra-plugin-gpu=true --overwrite
        # Label for status-updater topology (node pool name)
        kubectl label node "${NODE}" run.ai/simulated-gpu-node-pool=default --overwrite
    done
    
    # Store worker node name for later reference
    WORKER_NODE=$(kubectl get nodes -o jsonpath='{.items[?(@.metadata.labels.kubernetes\.io/role=="")].metadata.name}' | awk '{print $1}')
    if [[ -z "${WORKER_NODE}" ]]; then
        WORKER_NODE=$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')
    fi
    # Deploy fake-gpu-operator with DRA plugin, status-updater, topology-server, and status-exporter
    echo "Deploying fake-gpu-operator..."
    cd "${PROJECT_ROOT}"
    helm upgrade -i fake-gpu-operator deploy/fake-gpu-operator \
        --namespace gpu-operator \
        --create-namespace \
        -f "${SCRIPTS_DIR}/values.yaml" \
        --set draPlugin.image.tag="${DOCKER_TAG}" \
        --set statusUpdater.image.tag="${DOCKER_TAG}" \
        --set topologyServer.image.tag="${DOCKER_TAG}" \
        --set kwokDraPlugin.image.tag="${DOCKER_TAG}"

    echo "Waiting for status-updater pod to be ready..."
    kubectl wait --for=condition=Ready pod -l app=status-updater -n gpu-operator --timeout=120s

    echo "Waiting for topology-server pod to be ready..."
    kubectl wait --for=condition=Ready pod -l app=topology-server -n gpu-operator --timeout=120s

    echo "Waiting for DRA plugin pod to be ready..."
    kubectl wait --for=condition=Ready pod -l app.kubernetes.io/component=kubeletplugin -n gpu-operator --timeout=120s

    echo "Waiting for kwok-dra-plugin pod to be ready..."
    kubectl wait --for=condition=Ready pod -l app=kwok-dra-plugin -n gpu-operator --timeout=120s

    # Install KWOK controller for simulated nodes
    echo "Installing KWOK controller..."
    KWOK_VERSION="${KWOK_VERSION:-v0.7.0}"
    kubectl apply -f "https://github.com/kubernetes-sigs/kwok/releases/download/${KWOK_VERSION}/kwok.yaml"
    
    echo "Waiting for KWOK controller to be ready..."
    kubectl wait --for=condition=Ready pod -l app=kwok-controller -n kube-system --timeout=120s

    # Install KWOK stages for node heartbeat and pod lifecycle simulation
    echo "Installing KWOK stages..."
    kubectl apply -f "https://github.com/kubernetes-sigs/kwok/releases/download/${KWOK_VERSION}/stage-fast.yaml"

    # Create a KWOK simulated node with GPU topology
    echo "Creating KWOK simulated GPU node..."
    KWOK_NODE_NAME="kwok-gpu-node-1"
    
    # Create the KWOK node
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Node
metadata:
  annotations:
    kwok.x-k8s.io/node: fake
    node.alpha.kubernetes.io/ttl: "0"
  labels:
    kubernetes.io/role: worker
    node.kubernetes.io/instance-type: gpu-node
    run.ai/simulated-gpu-node-pool: default
    type: kwok
  name: ${KWOK_NODE_NAME}
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
  conditions:
  - lastHeartbeatTime: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    lastTransitionTime: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    message: kubelet has sufficient memory available
    reason: KubeletHasSufficientMemory
    status: "False"
    type: MemoryPressure
  - lastHeartbeatTime: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    lastTransitionTime: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    message: kubelet has no disk pressure
    reason: KubeletHasNoDiskPressure
    status: "False"
    type: DiskPressure
  - lastHeartbeatTime: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    lastTransitionTime: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    message: kubelet has sufficient PID available
    reason: KubeletHasSufficientPID
    status: "False"
    type: PIDPressure
  - lastHeartbeatTime: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    lastTransitionTime: "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    message: kubelet is posting ready status
    reason: KubeletReady
    status: "True"
    type: Ready
  nodeInfo:
    architecture: amd64
    containerRuntimeVersion: ""
    kernelVersion: ""
    kubeProxyVersion: fake
    kubeletVersion: fake
    operatingSystem: linux
    osImage: ""
EOF

    # The status-updater will automatically create a topology ConfigMap for the KWOK node
    # because it has the run.ai/simulated-gpu-node-pool label. The kwok.x-k8s.io/node annotation
    # is copied from the node to the ConfigMap by the status-updater.

    echo "Waiting for status-updater to create topology ConfigMap for KWOK node..."
    for i in {1..30}; do
        if kubectl get cm -n gpu-operator -l "run.ai/node-name=${KWOK_NODE_NAME}" >/dev/null 2>&1; then
            echo "Topology ConfigMap created by status-updater!"
            break
        fi
        echo "Waiting for topology ConfigMap... ($i/30)"
        sleep 2
    done

    # Wait for ResourceSlice to be created by kwok-dra-plugin
    echo "Waiting for ResourceSlice to be created for KWOK node..."
    for i in {1..30}; do
        if kubectl get resourceslice "kwok-${KWOK_NODE_NAME}-gpu" >/dev/null 2>&1; then
            echo "ResourceSlice created successfully!"
            break
        fi
        echo "Waiting for ResourceSlice... ($i/30)"
        sleep 2
    done

    echo "Setup complete! Cluster ${KIND_CLUSTER_NAME} is ready."
    echo "Worker node: ${WORKER_NODE}"
    echo "KWOK GPU node: ${KWOK_NODE_NAME}"
else
    echo "Skipping setup (SKIP_SETUP=true)"
fi
