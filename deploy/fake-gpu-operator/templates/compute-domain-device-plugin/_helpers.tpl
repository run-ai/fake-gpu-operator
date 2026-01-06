{{- define "fake-gpu-operator.compute-domain-device-plugin.common.metadata.labels" -}}
app: compute-domain-device-plugin
{{- end -}}

{{- define "fake-gpu-operator.compute-domain-device-plugin.common.metadata.annotations" -}}
openshift.io/scc: hostmount-anyuid
{{- end -}}

{{- define "fake-gpu-operator.compute-domain-device-plugin.common.metadata.name" -}}
compute-domain-device-plugin
{{- end -}}

{{- define "fake-gpu-operator.compute-domain-device-plugin.common.podSelector" }}
matchLabels:
  app: compute-domain-device-plugin
  component: compute-domain-device-plugin
{{- end }}

{{- define "fake-gpu-operator.compute-domain-device-plugin.common.podTemplate.metadata" }}
annotations:
  checksum/topology: {{ include (print $.Template.BasePath "/topology-cm.yml") . | sha256sum }}
labels:
  app: compute-domain-device-plugin
  component: compute-domain-device-plugin
{{- end }}

{{- define "fake-gpu-operator.compute-domain-device-plugin.common.podTemplate.spec" }}
containers:
  - image: "{{ .Values.computeDomainDevicePlugin.image.repository }}:{{ .Values.computeDomainDevicePlugin.image.tag | default .Chart.AppVersion }}"
    imagePullPolicy: "{{ .Values.computeDomainDevicePlugin.image.pullPolicy }}"
    resources:
      {{- toYaml .Values.computeDomainDevicePlugin.resources | nindent 12 }}
    env:
      - name: NODE_NAME
        valueFrom:
          fieldRef:
            fieldPath: spec.nodeName
      - name: CDI_ROOT
        value: "/etc/cdi"
      - name: KUBELET_REGISTRAR_DIRECTORY_PATH
        value: "/var/lib/kubelet/plugins_registry"
      - name: KUBELET_PLUGINS_DIRECTORY_PATH
        value: "/var/lib/kubelet/plugins"
    name: compute-domain-device-plugin-ctr
    securityContext:
      privileged: true
    terminationMessagePath: /dev/termination-log
    terminationMessagePolicy: File
    volumeMounts:
      - mountPath: /runai/bin
        name: runai-bin-directory
      - mountPath: /runai/shared
        name: runai-shared-directory
      - mountPath: /var/lib/kubelet/plugins_registry
        name: plugins-registry
      - mountPath: /var/lib/kubelet/plugins
        name: plugins
      - mountPath: /etc/cdi
        name: cdi
dnsPolicy: ClusterFirst
restartPolicy: Always
serviceAccountName: compute-domain-device-plugin
terminationGracePeriodSeconds: 30
tolerations:
  - effect: NoSchedule
    key: nvidia.com/gpu
    operator: Exists
imagePullSecrets:
  - name: gcr-secret
volumes:
  - hostPath:
      path: /var/lib/kubelet/plugins_registry
      type: DirectoryOrCreate
    name: plugins-registry
  - hostPath:
      path: /var/lib/kubelet/plugins
      type: DirectoryOrCreate
    name: plugins
  - hostPath:
      path: /etc/cdi
      type: DirectoryOrCreate
    name: cdi
  - hostPath:
      path: /var/lib/runai/bin
      type: DirectoryOrCreate
    name: runai-bin-directory
  - hostPath:
      path: /var/lib/runai/shared
      type: DirectoryOrCreate
    name: runai-shared-directory
{{- end }}