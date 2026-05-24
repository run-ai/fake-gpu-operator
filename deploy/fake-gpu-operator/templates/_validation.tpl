{{- /*
Chart-wide validations. Imported via `{{- template "fake-gpu-operator.validate" . }}`
at the top of any always-rendered manifest (see `topology-cm.yml`); failures
abort `helm template`/`helm install`/`helm upgrade` before any apiserver
call is made.

To add a check: append a new `{{- if ... }}{{- fail "..." -}}{{- end -}}`
block below.
*/ -}}
{{- define "fake-gpu-operator.validate" -}}

{{- /* statusExporter ⇄ upstream gpu-operator.dcgmExporter conflict.

Both produce a Service named `nvidia-dcgm-exporter` in the chart's
namespace. The upstream gpu-operator state-dcgm-exporter reconciler
owns that name and actively deletes any resource it doesn't manage, so
FGO's statusExporter DaemonSet+Service get garbage-collected within
seconds of creation. Fail fast at render time rather than silently
churn in the cluster. */ -}}
{{- if and .Values.statusExporter.enabled (.Values.gpuOperator).enabled -}}
{{- fail (printf "fake-gpu-operator: statusExporter and gpuOperator cannot both be enabled — both create a `nvidia-dcgm-exporter` Service in namespace %q and the upstream gpu-operator's reconciler deletes FGO's resources. Either set `statusExporter.enabled: false` (the upstream `gpu-operator.dcgmExporter` is enabled by default and provides the Service runai-cluster polls), or set `gpuOperator.enabled: false` to keep FGO's statusExporter." .Release.Namespace) -}}
{{- end -}}

{{- end -}}
