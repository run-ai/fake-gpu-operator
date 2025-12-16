{{/*
Expand the name of the chart.
*/}}
{{- define "dra-example-driver.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "dra-example-driver.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Allow the release namespace to be overridden for multi-namespace deployments in combined charts
*/}}
{{- define "dra-example-driver.namespace" -}}
  {{- if .Values.namespaceOverride -}}
    {{- .Values.namespaceOverride -}}
  {{- else -}}
    {{- .Release.Namespace -}}
  {{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "dra-example-driver.chart" -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- printf "%s-%s" $name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "dra-example-driver.labels" -}}
helm.sh/chart: {{ include "dra-example-driver.chart" . }}
{{ include "dra-example-driver.templateLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{ end -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{/*
Template labels
*/}}
{{- define "dra-example-driver.templateLabels" -}}
app.kubernetes.io/name: {{ include "dra-example-driver.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Values.selectorLabelsOverride }}
{{ toYaml .Values.selectorLabelsOverride }}
{{- end -}}
{{- end -}}

{{/*
Selector labels
*/}}
{{- define "dra-example-driver.selectorLabels" -}}
{{- if .Values.selectorLabelsOverride -}}
{{ toYaml .Values.selectorLabelsOverride }}
{{- else -}}
{{ include "dra-example-driver.templateLabels" . }}
{{- end -}}
{{- end -}}

{{/*
Full image name with tag
*/}}
{{- define "dra-example-driver.fullimage" -}}
{{- .Values.draPlugin.image.repository -}}:{{- .Values.draPlugin.image.tag | default .Chart.AppVersion -}}
{{- end -}}

{{/*
Create the name of the service account to use
*/}}
{{- define "dra-example-driver.serviceAccountName" -}}
{{- $name := printf "%s-service-account" (include "dra-example-driver.fullname" .) }}
{{- if .Values.draPlugin.serviceAccount.create }}
{{- default $name .Values.draPlugin.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.draPlugin.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Create the name of the service account to use for the webhook
*/}}
{{- define "dra-example-driver.webhookServiceAccountName" -}}
{{- $name := printf "%s-webhook-service-account" (include "dra-example-driver.fullname" .) }}
{{- if .Values.webhook.serviceAccount.create }}
{{- default $name .Values.webhook.serviceAccount.name }}
{{- else }}
{{- default "default-webhook" .Values.webhook.serviceAccount.name }}
{{- end -}}
{{- end -}}

{{/*
Get the latest available resource.k8s.io API version
Returns the highest available version or fails with error if none found
*/}}
{{- define "dra-example-driver.resourceApiVersion" -}}
{{- $apiVersion := "" -}}
{{- if .Capabilities.APIVersions.Has "resource.k8s.io/v1" -}}
{{- $apiVersion = "resource.k8s.io/v1" -}}
{{- else if .Capabilities.APIVersions.Has "resource.k8s.io/v1beta2" -}}
{{- $apiVersion = "resource.k8s.io/v1beta2" -}}
{{- else if .Capabilities.APIVersions.Has "resource.k8s.io/v1beta1" -}}
{{- $apiVersion = "resource.k8s.io/v1beta1" -}}
{{- end -}}
{{- required (printf "No supported resource.k8s.io API version found. This chart requires Kubernetes 1.30+ with Dynamic Resource Allocation feature gate enabled. Cluster API versions: %s" (.Capabilities.APIVersions | join ", ")) $apiVersion -}}
{{- end -}}