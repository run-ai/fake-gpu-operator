#!/usr/bin/env bash

set -e

# A reference to the current directory where this script is located
SCRIPTS_DIR="$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd)"
PROJECT_ROOT="$(cd -- "${SCRIPTS_DIR}/../.." &> /dev/null && pwd)"

# For e2e tests, we need to load images into docker (not push to registry)
# --load only works with single-platform builds, so we detect the current platform
CURRENT_PLATFORM="linux/$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')"
# The name of the kind cluster to create
: ${KIND_CLUSTER_NAME:="fake-gpu-operator-cluster"}

# Values file for Helm install (override to test profile-based config)
: ${VALUES_FILE:="${SCRIPTS_DIR}/values.yaml"}

# The path to kind's cluster configuration file
: ${KIND_CLUSTER_CONFIG_PATH:="${SCRIPTS_DIR}/kind-cluster-config.yaml"}

# Kubernetes version for kind
: ${KIND_K8S_TAG:="v1.34.0"}

# The name of the kind image to use
: ${KIND_IMAGE:="kindest/node:${KIND_K8S_TAG}"}

# Docker image tag
: ${DOCKER_TAG:="0.0.0-dev"}

# Check if docker is available
if ! command -v docker &> /dev/null; then
    echo "Docker not found. Please install Docker."
        exit 1
fi

echo "Docker found in PATH."

# Check if cluster already exists
if [[ "${SKIP_SETUP}" != "true" ]]; then
    if kind get clusters | grep -q "^${KIND_CLUSTER_NAME}$"; then
        echo "Cluster ${KIND_CLUSTER_NAME} already exists. Use SKIP_SETUP=true to skip setup or delete the cluster first."
        exit 1
    fi

    echo "Building Docker images for platform ${CURRENT_PLATFORM}..."
    cd "${PROJECT_ROOT}"
    # Use --load to load images into docker, with single platform (required for --load)
    make image DOCKER_TAG="${DOCKER_TAG}" DOCKER_BUILDX_PLATFORMS="${CURRENT_PLATFORM}" DOCKER_BUILDX_PUSH_FLAG="--load"

    echo "Creating kind cluster ${KIND_CLUSTER_NAME}..."
    kind create cluster \
        --name "${KIND_CLUSTER_NAME}" \
        --image "${KIND_IMAGE}" \
        --config "${KIND_CLUSTER_CONFIG_PATH}" \
        --wait 2m

    echo "Loading images into kind cluster..."
    DOCKER_REPO_BASE="${DOCKER_REPO_BASE:-ghcr.io/run-ai/fake-gpu-operator}"
    for component in dra-plugin-gpu status-updater status-exporter topology-server kwok-gpu-device-plugin kwok-dra-plugin kwok-compute-domain-dra-plugin compute-domain-controller compute-domain-dra-plugin; do
        IMAGE="${DOCKER_REPO_BASE}/${component}:${DOCKER_TAG}"
        echo "Loading ${IMAGE}..."
        kind load docker-image \
                --name "${KIND_CLUSTER_NAME}" \
                "${IMAGE}"
    done

    echo "Waiting for nodes to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=120s
    
    # Store worker node name for later reference
    WORKER_NODE=$(kubectl get nodes -o jsonpath='{.items[?(@.metadata.labels.kubernetes\.io/hostname!="")].metadata.name}' | awk '{print $1}')
    if [[ -z "${WORKER_NODE}" ]]; then
        WORKER_NODE=$(kubectl get nodes -o jsonpath='{.items[1].metadata.name}')
    fi
    echo "Worker node: ${WORKER_NODE}"
    # Install PrometheusRule CRD for runai e2e tests
    echo "Installing PrometheusRule CRD..."
    kubectl apply -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/main/example/prometheus-operator-crd/monitoring.coreos.com_prometheusrules.yaml

    # Deploy fake-gpu-operator with DRA plugin, status-updater, topology-server, and status-exporter
    echo "Deploying fake-gpu-operator..."
    cd "${PROJECT_ROOT}"
    helm upgrade -i fake-gpu-operator deploy/fake-gpu-operator \
        --namespace gpu-operator \
        --create-namespace \
        -f "${VALUES_FILE}" \
        --set draPlugin.image.tag="${DOCKER_TAG}" \
        --set statusUpdater.image.tag="${DOCKER_TAG}" \
        --set statusExporter.image.tag="${DOCKER_TAG}" \
        --set topologyServer.image.tag="${DOCKER_TAG}" \
        --set kwokDraPlugin.image.tag="${DOCKER_TAG}" \
        --set computeDomainController.image.tag="${DOCKER_TAG}" \
        --set computeDomainDraPlugin.image.tag="${DOCKER_TAG}" \
        --set kwokComputeDomainDraPlugin.image.tag="${DOCKER_TAG}" \
        --set kwokGpuDevicePlugin.image.tag="${DOCKER_TAG}" \
        --set statusUpdater.componentController.fallbackImageTag="${DOCKER_TAG}"

    echo "Waiting for status-updater pod to be ready..."
    kubectl wait --for=condition=Ready pod -l app=status-updater -n gpu-operator --timeout=120s

    echo "Waiting for status-exporter pod to be ready..."
    kubectl wait --for=condition=Ready pod -l app=nvidia-dcgm-exporter -n gpu-operator --timeout=120s

    echo "Waiting for topology-server pod to be ready..."
    kubectl wait --for=condition=Ready pod -l app=topology-server -n gpu-operator --timeout=120s

    echo "Waiting for DRA plugin pod to be ready..."
    kubectl wait --for=condition=Ready pod -l app.kubernetes.io/component=kubeletplugin -n gpu-operator --timeout=120s

    echo "Waiting for kwok-dra-plugin pod to be ready..."
    kubectl wait --for=condition=Ready pod -l app=kwok-dra-plugin -n gpu-operator --timeout=120s

    echo "Waiting for compute-domain-dra-plugin daemonset to be ready..."
    kubectl wait --for=condition=Ready pod -l app=compute-domain-dra-plugin -n gpu-operator --timeout=120s

    echo "Waiting for kwok-compute-domain-dra-plugin deployment to be ready..."
    kubectl wait --for=condition=Ready pod -l app=kwok-compute-domain-dra-plugin -n gpu-operator --timeout=120s

    # Install KWOK controller for simulated nodes
    echo "Installing KWOK controller..."
    KWOK_VERSION="${KWOK_VERSION:-v0.7.0}"
    kubectl apply -f "https://github.com/kubernetes-sigs/kwok/releases/download/${KWOK_VERSION}/kwok.yaml"
    
    echo "Waiting for KWOK controller to be ready..."
    kubectl wait --for=condition=Ready pod -l app=kwok-controller -n kube-system --timeout=120s

    # Install KWOK stages for node heartbeat and pod lifecycle simulation
    echo "Installing KWOK stages..."
    kubectl apply -f "https://github.com/kubernetes-sigs/kwok/releases/download/${KWOK_VERSION}/stage-fast.yaml"

    # Create KWOK simulated nodes with GPU topology
    # Nodes are split across pools: 1-3 → "default", 4-5 → "highend"
    echo "Creating KWOK simulated GPU nodes..."
    KWOK_NODES=("kwok-gpu-node-1" "kwok-gpu-node-2" "kwok-gpu-node-3" "kwok-gpu-node-4" "kwok-gpu-node-5")
    KWOK_NODE_TEMPLATE="${SCRIPTS_DIR}/kwok-node-template.yaml"

    # Returns pool name for a given node name (nodes 1-3 → default, 4-5 → highend)
    pool_for_node() {
        case "$1" in
            kwok-gpu-node-[123]) echo "default" ;;
            kwok-gpu-node-[45])  echo "highend" ;;
        esac
    }

    # Function to create a KWOK node from template
    create_kwok_node() {
        local NODE_NAME=$1
        local NODE_POOL=$2

        # Replace placeholders in template and apply
        sed -e "s/KWOK_NODE_NAME_PLACEHOLDER/${NODE_NAME}/g" \
            -e "s/KWOK_NODE_POOL_PLACEHOLDER/${NODE_POOL}/g" \
            "${KWOK_NODE_TEMPLATE}" | kubectl apply -f -
    }

    # Create all KWOK nodes with their assigned pools
    for NODE_NAME in "${KWOK_NODES[@]}"; do
        NODE_POOL=$(pool_for_node "${NODE_NAME}")
        echo "Creating KWOK node: ${NODE_NAME} (pool: ${NODE_POOL})..."
        create_kwok_node "${NODE_NAME}" "${NODE_POOL}"
    done

    # The status-updater will automatically create a topology ConfigMap for each KWOK node
    # because they have the run.ai/simulated-gpu-node-pool label. The kwok.x-k8s.io/node annotation
    # is copied from the node to the ConfigMap by the status-updater.

    # Wait for topology ConfigMaps to be created
    for NODE_NAME in "${KWOK_NODES[@]}"; do
        echo "Waiting for status-updater to create topology ConfigMap for ${NODE_NAME}..."
    for i in {1..30}; do
            if kubectl get cm -n gpu-operator -l "node-name=${NODE_NAME}" >/dev/null 2>&1; then
                echo "Topology ConfigMap created for ${NODE_NAME}!"
            break
        fi
        echo "Waiting for topology ConfigMap... ($i/30)"
        sleep 2
    done

    # Wait for ResourceSlice to be created by kwok-dra-plugin
        echo "Waiting for ResourceSlice to be created for ${NODE_NAME}..."
    for i in {1..30}; do
            if kubectl get resourceslice "kwok-${NODE_NAME}-gpu" >/dev/null 2>&1; then
                echo "ResourceSlice created for ${NODE_NAME}!"
            break
        fi
        echo "Waiting for ResourceSlice... ($i/30)"
        sleep 2
        done
    done

    # If component controller is enabled, wait for controller-managed deployments
    # The controller creates per-pool deployments after the topology CM is populated
    COMPONENT_CONTROLLER_ENABLED=$(helm get values fake-gpu-operator -n gpu-operator -o json 2>/dev/null | python3 -c "import sys,json; v=json.load(sys.stdin); print(v.get('statusUpdater',{}).get('componentController',{}).get('enabled',False))" 2>/dev/null || echo "False")
    if [[ "${COMPONENT_CONTROLLER_ENABLED}" == "True" ]]; then
        echo "Component controller is enabled, waiting for controller-managed deployments..."
        for i in {1..30}; do
            MANAGED_DEPS=$(kubectl get deployments -n gpu-operator -l "app.kubernetes.io/managed-by=fake-gpu-operator" --no-headers 2>/dev/null | wc -l | tr -d ' ')
            if [[ "${MANAGED_DEPS}" -ge 1 ]]; then
                echo "Found ${MANAGED_DEPS} controller-managed deployments!"
                kubectl wait --for=condition=Available deployment -l "app.kubernetes.io/managed-by=fake-gpu-operator" -n gpu-operator --timeout=120s
                break
            fi
            echo "Waiting for controller-managed deployments... ($i/30)"
            sleep 2
        done
    fi

    echo "Setup complete! Cluster ${KIND_CLUSTER_NAME} is ready."
    echo "Worker node: ${WORKER_NODE}"
    echo "KWOK GPU nodes: ${KWOK_NODES[*]}"
else
    echo "Skipping setup (SKIP_SETUP=true)"
fi
