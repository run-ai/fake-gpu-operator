{{/*
Get the image information for a component.
Usage: {{ include "fake-gpu-operator.fullImageName" (dict "component" .Values.devicePlugin "global" .Values.global "chart" .Chart) }}
*/}}
{{- define "fake-gpu-operator.fullImageName" -}}
{{- $component := .component -}}
{{- $global := .global -}}
{{- $chart := .chart -}}
{{- $repository := $component.repository | default $global.image.repository -}}
{{- $tag := $component.tag | default $global.image.tag | default $chart.AppVersion -}}
{{- $name := $component.name | default "" -}}
{{- printf "%s/%s:%s" $repository $name $tag -}}
{{- end -}}

{{/*
Get the image pull policy for a component.
Usage: {{ include "fake-gpu-operator.imagePullPolicy" (dict "component" .Values.devicePlugin "global" .Values.global) }}
*/}}
{{- define "fake-gpu-operator.imagePullPolicy" -}}
{{- $component := .component -}}
{{- $global := .global -}}
{{- $pullPolicy := $component.pullPolicy | default $global.image.pullPolicy -}}
{{- printf "%s" $pullPolicy -}}
{{- end -}}
