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

