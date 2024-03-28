{{- define "fake-gpu-operator.device-plugin.metadata" }}
metadata:
  {{- if .Values.environment.openshift }}
  annotations:
    openshift.io/scc: hostmount-anyuid
  {{- end }}
  labels:
    app: device-plugin
  name: device-plugin
  namespace: {{ .Release.Namespace }}
{{- end }}

{{- define "fake-gpu-operator.device-plugin.podSelector" }}
selector:
  matchLabels:
    app: device-plugin
    component: device-plugin
{{- end }}

{{- define "fake-gpu-operator.device-plugin.podTemplate.metadata" }}
metadata:
  annotations:
    checksum/initialTopology: {{ include (print $.Template.BasePath "/topology-cm.yml") . | sha256sum }}
  labels:
    app: device-plugin
    component: device-plugin
{{- end }}

{{- define "fake-gpu-operator.device-plugin.podTemplate.spec.common" }}
containers:
  - image: "{{ .Values.devicePlugin.image.repository }}:{{ .Values.devicePlugin.image.tag }}"
    imagePullPolicy: "{{ .Values.devicePlugin.image.pullPolicy }}"
    resources:
      {{- toYaml .Values.devicePlugin.resources | nindent 12 }}
    env:
      - name: NODE_NAME
        valueFrom:
          fieldRef:
            fieldPath: spec.nodeName
      - name: TOPOLOGY_CM_NAME
        value: topology
      - name: TOPOLOGY_CM_NAMESPACE
        value: "{{ .Release.Namespace }}"
    name: nvidia-device-plugin-ctr
    securityContext:
      privileged: true
    terminationMessagePath: /dev/termination-log
    terminationMessagePolicy: File
    volumeMounts:
      - mountPath: /runai/bin
        name: runai-bin-directory
      - mountPath: /runai/shared
        name: runai-shared-directory              
      - mountPath: /var/lib/kubelet/device-plugins
        name: device-plugin
dnsPolicy: ClusterFirst
restartPolicy: Always
serviceAccountName: nvidia-device-plugin
terminationGracePeriodSeconds: 30
tolerations:
  - effect: NoSchedule
    key: nvidia.com/gpu
    operator: Exists
imagePullSecrets:
  - name: gcr-secret
volumes:
  - hostPath:
      path: /var/lib/kubelet/device-plugins
      type: ""
    name: device-plugin
  - hostPath:
      path: /var/lib/runai/bin
      type: DirectoryOrCreate
    name: runai-bin-directory
  - hostPath:
      path: /var/lib/runai/shared
      type: DirectoryOrCreate
    name: runai-shared-directory
{{- end }}

{{- define "fake-gpu-operator.device-plugin.deployment" }}
apiVersion: apps/v1
kind: Deployment
{{- include "fake-gpu-operator.device-plugin.metadata" .}}
spec:
  replicas: 1
  {{- include "fake-gpu-operator.device-plugin.podSelector" . | nindent 2 }}
  template:
    {{- include "fake-gpu-operator.device-plugin.podTemplate.metadata" . | nindent 4 }}
    spec:
      {{- include "fake-gpu-operator.device-plugin.podTemplate.spec.common" . | nindent 6 }}
{{- end }}