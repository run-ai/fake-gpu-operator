{{- define "fake-gpu-operator.device-plugin.common.metadata.labels" -}}
app: device-plugin
{{- end -}}

{{- define "fake-gpu-operator.device-plugin.common.metadata.annotations" -}}
openshift.io/scc: hostmount-anyuid
{{- end -}}

{{- define "fake-gpu-operator.device-plugin.common.metadata.name" -}}
device-plugin
{{- end -}}

{{- define "fake-gpu-operator.device-plugin.common.podSelector" }}
matchLabels:
  app: device-plugin
  component: device-plugin
{{- end }}

{{- define "fake-gpu-operator.device-plugin.common.podTemplate.metadata" }}
annotations:
  checksum/topology: {{ include (print $.Template.BasePath "/topology-cm.yml") . | sha256sum }}
labels:
  app: device-plugin
  component: device-plugin
{{- end }}

{{- define "fake-gpu-operator.device-plugin.common.podTemplate.spec" }}
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
