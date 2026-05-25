{{- /* Chart-wide validations. Triggered by templates/validate.yaml. Add checks below. */ -}}
{{- define "fake-gpu-operator.validate" -}}

{{- /* statusExporter ⇄ gpu-operator.dcgmExporter both own the nvidia-dcgm-exporter Service. */ -}}
{{- if and .Values.statusExporter.enabled (.Values.gpuOperator).enabled -}}
{{- fail (printf "fake-gpu-operator: statusExporter and gpuOperator cannot both be enabled — both create a `nvidia-dcgm-exporter` Service in namespace %q and the upstream gpu-operator's reconciler deletes FGO's resources. Either set `statusExporter.enabled: false` (the upstream `gpu-operator.dcgmExporter` is enabled by default and provides the Service runai-cluster polls), or set `gpuOperator.enabled: false` to keep FGO's statusExporter." .Release.Namespace) -}}
{{- end -}}

{{- end -}}
