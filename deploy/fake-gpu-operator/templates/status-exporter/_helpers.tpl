{{- define "fake-gpu-operator.status-exporter.metadata" }}
metadata:
  labels:
    app: nvidia-dcgm-exporter
    component: status-exporter
    app.kubernetes.io/name: nvidia-container-toolkit
  name: nvidia-dcgm-exporter
  namespace: {{ .Release.Namespace }}
{{- end }}

{{- define "fake-gpu-operator.status-exporter.podSelector" }}
selector:
  matchLabels:
    app: nvidia-dcgm-exporter
{{- end }}

{{- define "fake-gpu-operator.status-exporter.podTemplate.metadata" }}
metadata:
  creationTimestamp: null
  labels:
    app: nvidia-dcgm-exporter
    app.kubernetes.io/name: nvidia-container-toolkit
{{- end }}

{{- define "fake-gpu-operator.status-exporter.podTemplate.spec.common" }}
containers:
- image: "{{ .Values.statusExporter.image.repository }}:{{ .Values.statusExporter.image.tag }}"
  imagePullPolicy: "{{ .Values.statusExporter.image.pullPolicy }}"
  resources:
    {{- toYaml .Values.statusExporter.resources | nindent 8 }}
  name: nvidia-dcgm-exporter
  env:
    - name: NODE_NAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    - name: TOPOLOGY_CM_NAME
      value: topology
    - name: TOPOLOGY_CM_NAMESPACE
      value: "{{ .Release.Namespace }}"
    - name: TOPOLOGY_MAX_EXPORT_INTERVAL
      value: "{{ .Values.statusExporter.topologyMaxExportInterval }}"
  ports:
    - containerPort: 9400
      name: http
  volumeMounts:
    - mountPath: /runai/proc
      name: runai-proc-directory
restartPolicy: Always
schedulerName: default-scheduler
serviceAccount: status-exporter
serviceAccountName: status-exporter
tolerations:
  - effect: NoSchedule
    key: nvidia.com/gpu
    operator: Exists
imagePullSecrets:
  - name: gcr-secret
volumes:
  - name: runai-proc-directory
    hostPath:
      path: /var/lib/runai/proc
      type: DirectoryOrCreate
{{- end }}

{{- define "fake-gpu-operator.status-exporter.deployment" }}
apiVersion: apps/v1
kind: Deployment
{{- include "fake-gpu-operator.status-exporter.metadata" .}}
spec:
  replicas: 1
  {{- include "fake-gpu-operator.status-exporter.podSelector" . | nindent 2 }}
  template:
    {{- include "fake-gpu-operator.status-exporter.podTemplate.metadata" . | nindent 4 }}
    spec:
      {{- include "fake-gpu-operator.status-exporter.podTemplate.spec.common" . | nindent 6 }}
{{- end }}