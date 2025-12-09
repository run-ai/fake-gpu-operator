#!/usr/bin/env bash

# Copyright 2025 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -e

# A reference to the current directory where this script is located
SCRIPTS_DIR="$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd)"
PROJECT_ROOT="$(cd -- "${SCRIPTS_DIR}/../.." &> /dev/null && pwd)"

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

    echo "Building Docker images..."
    cd "${PROJECT_ROOT}"
    make image DOCKER_TAG="${DOCKER_TAG}" SHOULD_PUSH=false

    echo "Creating kind cluster ${KIND_CLUSTER_NAME}..."
    ${KIND} create cluster \
        --name "${KIND_CLUSTER_NAME}" \
        --image "${KIND_IMAGE}" \
        --config "${KIND_CLUSTER_CONFIG_PATH}" \
        --wait 2m

    echo "Loading images into kind cluster..."
    DOCKER_REPO_BASE="${DOCKER_REPO_BASE:-ghcr.io/run-ai/fake-gpu-operator}"
    for component in dra-plugin-gpu; do
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

    echo "Annotating all nodes with fake GPU devices..."
    GPU_ANNOTATION='{"gpuMemory":40960,"gpuProduct":"NVIDIA-A100-SXM4-40GB","gpus":[{"id":"GPU-12345678-1234-1234-1234-123456789abc","status":{"allocatedBy":{"namespace":"","pod":"","container":""},"podGpuUsageStatus":{}}},{"id":"GPU-87654321-4321-4321-4321-cba987654321","status":{"allocatedBy":{"namespace":"","pod":"","container":""},"podGpuUsageStatus":{}}}],"migStrategy":"none"}'
    
    # Get all node names
    NODES=$(kubectl get nodes -o jsonpath='{.items[*].metadata.name}')
    for NODE in ${NODES}; do
        echo "Annotating node ${NODE}..."
        kubectl annotate node "${NODE}" nvidia.com/gpu.fake.devices="${GPU_ANNOTATION}" --overwrite
    done
    
    # Store worker node name for later reference
    WORKER_NODE=$(kubectl get nodes -o jsonpath='{.items[?(@.metadata.labels.kubernetes\.io/role=="")].metadata.name}' | awk '{print $1}')
    if [[ -z "${WORKER_NODE}" ]]; then
        WORKER_NODE=$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')
    fi

    echo "Deploying fake-gpu-operator..."
    cd "${PROJECT_ROOT}"
    helm upgrade -i fake-gpu-operator deploy/fake-gpu-operator \
        --namespace gpu-operator \
        --create-namespace \
        --set draPlugin.enabled=true \
        --set draPlugin.image.tag="${DOCKER_TAG}" \
        --set draPlugin.image.pullPolicy=Never \
        --set devicePlugin.enabled=false \
        --set statusUpdater.enabled=false \
        --set topologyServer.enabled=false \
        --set statusExporter.enabled=false \
        --set kwokGpuDevicePlugin.enabled=false \
        --set migFaker.enabled=false \
        --set gpuOperator.enabled=false \
        --set runtimeClass.enabled=false \
        --set topologyConfigMap.enabled=false

    echo "Waiting for DRA plugin pod to be ready..."
    kubectl wait --for=condition=Ready pod -l app.kubernetes.io/component=kubeletplugin -n gpu-operator --timeout=120s

    echo "Setup complete! Cluster ${KIND_CLUSTER_NAME} is ready."
    echo "Worker node: ${WORKER_NODE}"
else
    echo "Skipping setup (SKIP_SETUP=true)"
fi

