#!/usr/bin/env bash

# Applies the HEAD chart on top of an already-installed baseline release.
# Runs independently of setup.sh so an ad-hoc iteration loop can re-run it
# after each chart edit without re-doing the cluster + baseline install.
#
# Captures the pre-upgrade topology ConfigMap UID into a marker file so the
# Ginkgo suite can verify the CM was patched in place (UID unchanged), not
# deleted+recreated, by the upgrade.

set -euo pipefail

SCRIPTS_DIR="$(cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd)"
PROJECT_ROOT="$(cd -- "${SCRIPTS_DIR}/../../../.." &> /dev/null && pwd)"
FIXTURES_DIR="$(cd -- "${SCRIPTS_DIR}/../fixtures" &> /dev/null && pwd)"

: ${KIND_CLUSTER_NAME:="upgrade-cluster"}
: ${VALUES_FILE:="${FIXTURES_DIR}/values-upgrade.yaml"}
: ${DOCKER_TAG:="0.0.0-dev"}
: ${RELEASE_NAME:="fake-gpu-operator"}
: ${RELEASE_NAMESPACE:="gpu-operator"}
: ${MARKER_FILE:="/tmp/${KIND_CLUSTER_NAME}-pre-upgrade-cm-uid"}

if ! kind get clusters | grep -q "^${KIND_CLUSTER_NAME}$"; then
    echo "ERROR: cluster ${KIND_CLUSTER_NAME} not found. Run 'make setup-e2e-upgrade' first."
    exit 1
fi
kubectl config use-context "kind-${KIND_CLUSTER_NAME}" >/dev/null

echo "==> Capturing pre-upgrade topology ConfigMap UID..."
PRE_UID=$(kubectl get cm -n "${RELEASE_NAMESPACE}" topology -o jsonpath='{.metadata.uid}')
if [[ -z "${PRE_UID}" ]]; then
    echo "ERROR: topology ConfigMap not found in ${RELEASE_NAMESPACE} — has the baseline been installed?"
    exit 1
fi
echo "${PRE_UID}" > "${MARKER_FILE}"
echo "    pre-upgrade UID: ${PRE_UID}"
echo "    marker file:     ${MARKER_FILE}"

echo "==> Resolving subchart deps for the local chart..."
cd "${PROJECT_ROOT}"
helm dependency update deploy/fake-gpu-operator

echo "==> Upgrading ${RELEASE_NAME} to the local HEAD chart (tag ${DOCKER_TAG})..."
helm upgrade "${RELEASE_NAME}" deploy/fake-gpu-operator \
    --namespace "${RELEASE_NAMESPACE}" \
    -f "${VALUES_FILE}" \
    --set devicePlugin.image.tag="${DOCKER_TAG}" \
    --set statusUpdater.image.tag="${DOCKER_TAG}" \
    --set topologyServer.image.tag="${DOCKER_TAG}" \
    --wait --timeout 5m

echo ""
echo "===================================================="
echo "Upgrade applied. Run 'make test-e2e-upgrade' to verify."
echo "===================================================="
