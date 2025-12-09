#!/usr/bin/env bash


set -e

# The name of the kind cluster to delete
: ${KIND_CLUSTER_NAME:="fake-gpu-operator-cluster"}

# Container tool, e.g. docker/podman
if [[ -z "${CONTAINER_TOOL}" ]]; then
    if [[ -n "$(which docker)" ]]; then
        CONTAINER_TOOL=docker
    elif [[ -n "$(which podman)" ]]; then
        CONTAINER_TOOL=podman
    else
        echo "No container tool detected. Please install Docker or Podman."
        exit 1
    fi
fi

: ${KIND:="env KIND_EXPERIMENTAL_PROVIDER=${CONTAINER_TOOL} kind"}

if [[ "${SKIP_TEARDOWN}" == "true" ]]; then
    echo "Skipping teardown (SKIP_TEARDOWN=true)"
    exit 0
fi

echo "Deleting kind cluster ${KIND_CLUSTER_NAME}..."
${KIND} delete cluster --name "${KIND_CLUSTER_NAME}"

echo "Teardown complete!"

