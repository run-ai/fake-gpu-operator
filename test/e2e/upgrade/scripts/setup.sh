#!/usr/bin/env bash

# Brings up a KIND cluster, builds + loads local component images for the
# eventual upgrade step, then installs the published OCI baseline release.
# The Ginkgo suite then runs `helm upgrade` against the local chart and
# asserts the upgrade succeeds — exercising the class of bug where a new
# top-level chart value is referenced unsafely in a template (RUN-39195).

set -euo pipefail

SCRIPTS_DIR="$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd)"
PROJECT_ROOT="$(cd -- "${SCRIPTS_DIR}/../../../.." &> /dev/null && pwd)"
FIXTURES_DIR="$(cd -- "${SCRIPTS_DIR}/../fixtures" &> /dev/null && pwd)"

CURRENT_PLATFORM="linux/$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')"

# Baseline chart version. Pinned to a known-good published release so the
# upgrade path is deterministic and reproducible — bumping this is an
# intentional change, not a side-effect of "newest available on GHCR".
: ${BASELINE_CHART_VERSION:="0.0.80"}

: ${KIND_CLUSTER_NAME:="upgrade-cluster"}
: ${VALUES_FILE:="${FIXTURES_DIR}/values-upgrade.yaml"}
: ${KIND_CLUSTER_CONFIG_PATH:="${FIXTURES_DIR}/kind-cluster-config.yaml"}
: ${KIND_K8S_TAG:="v1.34.2"}
: ${KIND_IMAGE:="kindest/node:${KIND_K8S_TAG}"}
: ${DOCKER_TAG:="0.0.0-dev"}
: ${DOCKER_REPO_BASE:="ghcr.io/run-ai/fake-gpu-operator"}
: ${BASELINE_CHART_REF:="oci://ghcr.io/run-ai/fake-gpu-operator/fake-gpu-operator"}
: ${RELEASE_NAME:="fake-gpu-operator"}
: ${RELEASE_NAMESPACE:="gpu-operator"}

if ! command -v docker &> /dev/null; then
    echo "Docker not found. Please install Docker."
    exit 1
fi

if [[ "${SKIP_SETUP:-false}" == "true" ]]; then
    echo "Skipping setup (SKIP_SETUP=true)"
    exit 0
fi

if kind get clusters | grep -q "^${KIND_CLUSTER_NAME}$"; then
    echo "Cluster ${KIND_CLUSTER_NAME} already exists. Use SKIP_SETUP=true or delete the cluster first."
    exit 1
fi

echo "==> Building local Docker images (platform ${CURRENT_PLATFORM}, tag ${DOCKER_TAG})..."
# Built once here so the eventual `helm upgrade` (run by the Ginkgo suite)
# finds the local images already loaded into kind.
cd "${PROJECT_ROOT}"
make image DOCKER_TAG="${DOCKER_TAG}" DOCKER_BUILDX_PLATFORMS="${CURRENT_PLATFORM}" DOCKER_BUILDX_PUSH_FLAG="--load"

echo "==> Creating kind cluster ${KIND_CLUSTER_NAME}..."
kind create cluster \
    --name "${KIND_CLUSTER_NAME}" \
    --image "${KIND_IMAGE}" \
    --config "${KIND_CLUSTER_CONFIG_PATH}" \
    --wait 2m

echo "==> Loading local FGO images into kind..."
# Only the components enabled by values-upgrade.yaml — devicePlugin,
# statusUpdater, topologyServer. Loading the rest wastes time.
for component in device-plugin status-updater topology-server; do
    IMAGE="${DOCKER_REPO_BASE}/${component}:${DOCKER_TAG}"
    echo "    -- ${IMAGE}"
    kind load docker-image --name "${KIND_CLUSTER_NAME}" "${IMAGE}"
done

echo "==> Installing baseline chart ${BASELINE_CHART_REF} version ${BASELINE_CHART_VERSION}..."
# pullPolicy: IfNotPresent in values-upgrade.yaml; baseline pulls from GHCR
# (public, no auth needed), local upgrade later reuses kind-loaded images.
helm install "${RELEASE_NAME}" "${BASELINE_CHART_REF}" \
    --version "${BASELINE_CHART_VERSION}" \
    --namespace "${RELEASE_NAMESPACE}" \
    --create-namespace \
    -f "${VALUES_FILE}" \
    --wait --timeout 5m

echo "==> Baseline release info:"
helm list --namespace "${RELEASE_NAMESPACE}"

echo ""
echo "===================================================="
echo "Setup complete!"
echo "  Cluster: ${KIND_CLUSTER_NAME}"
echo "  Baseline: ${RELEASE_NAME} @ ${BASELINE_CHART_VERSION}"
echo "  Local images loaded for upgrade step (tag: ${DOCKER_TAG})"
echo "===================================================="
