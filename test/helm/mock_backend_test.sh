#!/usr/bin/env bash
# Helm chart-render assertions for Phase 5 mock-backend support (RUN-38195).
# Runs `helm template` with various toggle combinations and asserts the
# rendered manifest contains/lacks expected resource shapes.
#
# Subchart detection patterns chosen from Step 1 exploration:
#   gpu-operator subchart:      "^kind: ClusterPolicy" — the ClusterPolicy CR is only
#                               emitted by the real gpu-operator subchart; the polyfill
#                               CRD definition does NOT emit this resource kind at the
#                               top level.
#   nvidia-dra-driver-gpu:      "nvidia-dra-driver-gpu-kubelet-plugin" — a DaemonSet
#                               name that only exists when the DRA subchart renders.
#
# NOTE: assert functions write rendered output to a temp file before grepping.
# macOS BSD grep silently truncates very large strings piped via `echo "$var"`,
# so file-based grep is required for correctness on macOS.
set -euo pipefail

CHART="${CHART:-deploy/fake-gpu-operator}"
PASS=0
FAIL=0

# Temp file for rendered manifests — reused across assertions within one case.
TMPFILE=$(mktemp)
trap 'rm -f "$TMPFILE"' EXIT

assert_contains() {
    local label="$1"
    local pattern="$2"
    # $TMPFILE already holds the rendered manifest for the current case.
    if grep -qE "$pattern" "$TMPFILE"; then
        echo "PASS: $label"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $label  (pattern not found: $pattern)"
        FAIL=$((FAIL + 1))
    fi
}

assert_absent() {
    local label="$1"
    local pattern="$2"
    if grep -qE "$pattern" "$TMPFILE"; then
        echo "FAIL: $label  (unexpected match: $pattern)"
        FAIL=$((FAIL + 1))
    else
        echo "PASS: $label"
        PASS=$((PASS + 1))
    fi
}

# assert_next_line_absent checks that the line immediately following a grep
# match does NOT contain a given sub-string.  Used to verify that the
# "  driver:" YAML key is NOT followed by "enabled: true" (i.e. our mandatory
# override driver.enabled=false is respected).
assert_next_line_absent() {
    local label="$1"
    local anchor_pattern="$2"   # grep -E pattern that must match the "parent" line
    local bad_fragment="$3"     # literal string that must NOT appear on the next line
    if grep -A1 "$anchor_pattern" "$TMPFILE" | grep -qF "$bad_fragment"; then
        echo "FAIL: $label  (found '$bad_fragment' after '$anchor_pattern')"
        FAIL=$((FAIL + 1))
    else
        echo "PASS: $label"
        PASS=$((PASS + 1))
    fi
}

# render renders the chart with given extra args and writes output to TMPFILE.
render() {
    helm template fgo "$CHART" "$@" > "$TMPFILE"
}

# render_dra renders with --namespace gpu-operator to satisfy the DRA subchart's
# built-in validation that rejects the default namespace.
render_dra() {
    helm template fgo "$CHART" --namespace gpu-operator "$@" > "$TMPFILE"
}

echo "=== Case 1: defaults (both toggles false) ==="
render
# Polyfill placeholder Deployment: replicas 0 running ubuntu:22.04
assert_contains "polyfill placeholder Deployment renders" "image: ubuntu:22.04"
assert_contains "polyfill ClusterPolicy CRD renders"      "kind: CustomResourceDefinition"
# GPU operator subchart absent: ClusterPolicy CR would only appear if subchart ran
assert_absent   "GPU Operator subchart absent"            "^kind: ClusterPolicy"
# DRA subchart absent: kubelet-plugin DaemonSet name unique to that subchart
assert_absent   "DRA driver subchart absent"              "nvidia-dra-driver-gpu-kubelet-plugin"
# nvml-mock ServiceAccount gated on either subchart toggle being true
assert_absent   "nvml-mock SA absent"                     "name: nvml-mock$"

echo "=== Case 2: gpuOperator.enabled=true, nvidiaDraDriver.enabled=false ==="
render --set gpuOperator.enabled=true
assert_absent   "polyfill suppressed when subchart on"    "image: ubuntu:22.04"
assert_contains "GPU Operator subchart present"           "^kind: ClusterPolicy"
assert_absent   "DRA driver subchart absent"              "nvidia-dra-driver-gpu-kubelet-plugin"
assert_contains "nvml-mock SA present"                    "name: nvml-mock$"
assert_contains "NVML_MOCK_IMAGE env present"             "NVML_MOCK_IMAGE"

echo "=== Case 3: gpuOperator.enabled=false, nvidiaDraDriver.enabled=true ==="
render_dra --set nvidiaDraDriver.enabled=true
assert_contains "polyfill still rendered (real GPU Op absent)" "image: ubuntu:22.04"
assert_contains "DRA driver subchart present"             "nvidia-dra-driver-gpu-kubelet-plugin"
assert_contains "nvml-mock SA present"                    "name: nvml-mock$"

echo "=== Case 4: both toggles true ==="
render_dra --set gpuOperator.enabled=true --set nvidiaDraDriver.enabled=true
assert_absent   "polyfill suppressed"                     "image: ubuntu:22.04"
assert_contains "GPU Operator subchart present"           "^kind: ClusterPolicy"
assert_contains "DRA driver subchart present"             "nvidia-dra-driver-gpu-kubelet-plugin"

echo "=== Case 5: user override under gpu-operator: subchart key wins ==="
# Verify a user override on a non-mandatory key (devicePlugin.repository) flows
# through without breaking our mandatory overrides like driver.enabled=false.
render --set gpuOperator.enabled=true --set 'gpu-operator.devicePlugin.repository=myorg/dp'
assert_contains "user override flows through"             "repository: myorg/dp"
# Mandatory override: values.yaml sets gpu-operator.driver.enabled=false.
# In the rendered ClusterPolicy the top-level "  driver:" YAML key must be
# immediately followed by "enabled: false", NOT "enabled: true".
assert_contains         "mandatory driver section present in ClusterPolicy" "^  driver:$"
assert_next_line_absent "mandatory driver.enabled=false preserved"          "^  driver:$" "enabled: true"

echo "=== Case 6: OCP CSV is independent of either toggle ==="
render --set environment.openshift=true
assert_contains "OCP CSV with default toggles"            "kind: ClusterServiceVersion"

render --set environment.openshift=true --set gpuOperator.enabled=true
assert_contains "OCP CSV with subchart on"                "kind: ClusterServiceVersion"

echo
echo "=== Summary: $PASS passed, $FAIL failed ==="
[ "$FAIL" -eq 0 ]
