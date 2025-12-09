#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# Establish base directory for the script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "${SCRIPT_DIR}")"

readonly CLUSTER_NAME="fake-gpu-operator"
readonly REGISTRY_NAMESPACE="kube-registry"
readonly REGISTRY_PORT="30100"
readonly LOCAL_REGISTRY="localhost:${REGISTRY_PORT}"

main() {
    setup_kind_cluster
    setup_local_registry
    configure_registry_access
    build_and_push_images
    deploy_with_helm
}

setup_kind_cluster() {
    echo "Setting up Kind cluster..."
    
    # Check if cluster already exists
    if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
        echo "Cluster ${CLUSTER_NAME} already exists, using existing cluster"
    else
        echo "Creating new Kind cluster..."
        kind create cluster --name "${CLUSTER_NAME}" --config "${PROJECT_ROOT}/hack/kind-config.yaml"
    fi
    
    kubectl cluster-info --context "kind-${CLUSTER_NAME}"
}

setup_local_registry() {
    echo "Setting up local registry..."
    kubectl apply -f "${PROJECT_ROOT}/hack/local_registry.yaml"
    
    echo "Waiting for registry to be ready..."
    kubectl wait --namespace "${REGISTRY_NAMESPACE}" \
        --for=condition=ready pod \
        --selector=app=registry \
        --timeout=90s
}

configure_registry_access() {
    echo "Configuring registry access with port-forward..."
    
    # Port forward the registry service
    kubectl port-forward -n "${REGISTRY_NAMESPACE}" svc/registry "${REGISTRY_PORT}:5000" &
    PORT_FORWARD_PID=$!
    
    # Give it a moment to establish
    sleep 1
    
    # Verify we can reach the registry
    if ! curl -s "http://${LOCAL_REGISTRY}/v2/_catalog" >/dev/null; then
        echo "Failed to access registry at ${LOCAL_REGISTRY}"
        kill $PORT_FORWARD_PID || true
        exit 1
    fi
    
    echo "Registry accessible at ${LOCAL_REGISTRY}"
}

build_and_push_images() {
    echo "Building and pushing images to local registry..."
    
    # Update DOCKER_REPO_BASE to point to our local registry
    export DOCKER_REPO_BASE="${LOCAL_REGISTRY}"
    # Use 0.0.0-test as default tag, but allow user override via DOCKER_TAG env var
    export DOCKER_TAG="${DOCKER_TAG:-0.0.0-test}"
    
    # Build images with push flag
    make -C "${PROJECT_ROOT}" docker-push
    
    echo "Images pushed to local registry successfully"
}

deploy_with_helm() {
    echo "Packaging and deploying with Helm..."
    
    # Navigate to helm chart directory
    local helm_chart_dir="${PROJECT_ROOT}/deploy/fake-gpu-operator"
    if [ ! -d "${helm_chart_dir}" ]; then
        echo "Helm chart directory not found at ${helm_chart_dir}"
        exit 1
    fi
    
    cd "${helm_chart_dir}"
    
    # Update values to use local registry
    echo "Setting LOCAL_REGISTRY to: ${LOCAL_REGISTRY}"
    echo "Setting DOCKER_TAG to: ${DOCKER_TAG}"
    
    # Use yq with explicit value assignment
    yq eval ".global.image.repository = \"${LOCAL_REGISTRY}\"" -i values.yaml
    yq eval ".global.image.tag = \"${DOCKER_TAG}\"" -i values.yaml
    
    # Verify the changes
    echo "Updated values:"
    yq eval '.global.image' values.yaml
    
    # Add required dependencies
    helm repo add ingress-nginx "https://kubernetes.github.io/ingress-nginx"
    helm repo update
    helm dependency update .
    
    # Install the chart
    helm upgrade --install fake-gpu-operator . \
        --namespace gpu-operator \
        --create-namespace \
        --wait \
        --timeout 30s
    
    echo "Deployment completed successfully!"
}

cleanup() {
    if [ -n "${PORT_FORWARD_PID:-}" ]; then
        kill "${PORT_FORWARD_PID}" || true
    fi
}

trap cleanup EXIT

log_error() {
    printf '\e[31mERROR: %s\n\e[39m' "$1" >&2
}

main "$@"
