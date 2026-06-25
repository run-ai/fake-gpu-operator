#!/usr/bin/env bash
# Helm chart-render assertions for OpenShift SCC support.
# Verifies that environment.openshift=true adds the privileged SCC rule
# to all component ClusterRoles that run privileged containers.
set -euo pipefail

CHART="${CHART:-deploy/fake-gpu-operator}"
PASS=0
FAIL=0

TMPFILE=$(mktemp)
trap 'rm -f "$TMPFILE"' EXIT

assert_contains() {
    local label="$1"
    local pattern="$2"
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

# Count occurrences of a pattern and assert the expected number.
assert_count() {
    local label="$1"
    local pattern="$2"
    local expected="$3"
    local actual
    actual=$(grep -cE "$pattern" "$TMPFILE" || true)
    if [ "$actual" -eq "$expected" ]; then
        echo "PASS: $label (count=$actual)"
        PASS=$((PASS + 1))
    else
        echo "FAIL: $label  (expected $expected, got $actual)"
        FAIL=$((FAIL + 1))
    fi
}

render() {
    helm template fgo "$CHART" "$@" > "$TMPFILE"
}

SCC_PATTERN="securitycontextconstraints"

echo "=== Case 1: defaults (environment.openshift=false) ==="
render
assert_absent "no SCC rules without openshift flag" "$SCC_PATTERN"

echo ""
echo "=== Case 2: environment.openshift=true, default components ==="
render --set environment.openshift=true
# status-exporter already had SCC — now device-plugin and mig-faker should too
assert_contains "SCC rule present in rendered output" "$SCC_PATTERN"
# device-plugin + status-exporter + mig-faker + gpu-operator subchart = 4
assert_count "SCC rule in 4 ClusterRoles" "$SCC_PATTERN" 4

echo ""
echo "=== Case 3: environment.openshift=true, DRA enabled ==="
render --set environment.openshift=true \
       --set draPlugin.enabled=true \
       --set computeDomainDraPlugin.enabled=true \
       --namespace gpu-operator
# device-plugin + status-exporter + mig-faker + dra-plugin + compute-domain-dra + gpu-operator subchart = 6
assert_count "SCC rule in 6 ClusterRoles" "$SCC_PATTERN" 6

echo ""
echo "=== Case 4: environment.openshift=true, only device-plugin ==="
render --set environment.openshift=true \
       --set statusExporter.enabled=false \
       --set migFaker.enabled=false
# device-plugin + gpu-operator subchart = 2
assert_count "SCC rule in 2 ClusterRoles" "$SCC_PATTERN" 2

echo ""
echo "--- Results: $PASS passed, $FAIL failed ---"
[ "$FAIL" -eq 0 ]
