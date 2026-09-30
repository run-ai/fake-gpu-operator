#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd -- "${SCRIPT_DIR}/../../../.." && pwd)"

export KIND_CLUSTER_NAME="${KIND_CLUSTER_NAME:-fake-gpu-operator-kai}"
"${PROJECT_ROOT}/test/e2e/scripts/teardown.sh"
