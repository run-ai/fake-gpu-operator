#!/usr/bin/env bash
# Render the chart with each top-level value nulled -- the state that a
# `helm upgrade --reuse-values` from an older release (or a parent chart setting
# `<key>: null`) leaves it in -- and fail if any render aborts with a Go-template
# "nil pointer" error. A null value should make a `{{- if .Values.x.enabled -}}`
# gate evaluate falsey and skip that section, not abort the whole `helm upgrade`.
# Fix offenders with the nil-safe `(.Values.x).field` access form.
set -u

CHART_SRC="${CHART_SRC:-deploy/fake-gpu-operator}"
command -v helm >/dev/null 2>&1 || { echo "chart-render-guard: helm not found in PATH"; exit 2; }

work="$(mktemp -d)"; trap 'rm -rf "$work"' EXIT
cp -R "$CHART_SRC" "$work/chart"

# The NGC subchart deps aren't vendored, so `helm template` refuses to render
# until they're present (there is no flag to skip that check). They're disabled
# in every scenario here, so drop the dependencies stanza -- the last block in
# Chart.yaml -- to keep the guard hermetic and fast (no NGC pulls).
sed -i.bak '/^dependencies:/,$d' "$work/chart/Chart.yaml"; rm -f "$work/chart/Chart.yaml.bak"

fail=0
check() { # description, extra `helm template` args (e.g. --set foo=null)
  local desc="$1"; shift
  if helm template guard "$work/chart" --api-versions resource.k8s.io/v1 "$@" 2>&1 | grep -q "nil pointer"; then
    echo "  ✗ ${desc} -- nil-pointer render abort"; fail=1
  else
    echo "  ✓ ${desc}"
  fi
}

keys=$(grep -oE '^[a-zA-Z][a-zA-Z0-9_]*' "$work/chart/values.yaml" | sort -u)
[ -n "$keys" ] || { echo "chart-render-guard: found no top-level values -- discovery broke"; exit 2; }

echo "chart-render-guard: rendering ${CHART_SRC} with each top-level value nulled"
for k in $keys; do
  check "${k}: null" --set "${k}=null"
done
# Reported regression: a DRA plugin enabled while its subchart-condition key is null.
check "draPlugin.enabled + nvidiaDraDriver: null"     --set draPlugin.enabled=true     --set nvidiaDraDriver=null
check "kwokDraPlugin.enabled + nvidiaDraDriver: null" --set kwokDraPlugin.enabled=true --set nvidiaDraDriver=null

[ "$fail" -eq 0 ] && { echo "chart-render-guard: PASS"; exit 0; }
echo "chart-render-guard: FAIL -- use the nil-safe (.Values.x).field access form"; exit 1
