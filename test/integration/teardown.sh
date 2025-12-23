#!/usr/bin/env bash

set -e

# The name of the kind cluster to delete
: ${KIND_CLUSTER_NAME:="fake-gpu-operator-cluster"}

# Check if docker is available
if ! command -v docker &> /dev/null; then
    echo "Docker not found. Please install Docker."
    exit 1
fi

if [[ "${SKIP_TEARDOWN}" == "true" ]]; then
    echo "Skipping teardown (SKIP_TEARDOWN=true)"
    exit 0
fi

echo "Deleting kind cluster ${KIND_CLUSTER_NAME}..."
kind delete cluster --name "${KIND_CLUSTER_NAME}"

echo "Teardown complete!"

