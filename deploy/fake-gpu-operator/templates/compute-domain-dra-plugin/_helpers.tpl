{{- define "fake-gpu-operator.compute-domain-dra-plugin.common.metadata.labels" -}}
app: compute-domain-dra-plugin
{{- end -}}

{{- define "fake-gpu-operator.compute-domain-dra-plugin.common.metadata.name" -}}
compute-domain-dra-plugin
{{- end -}}

{{- define "fake-gpu-operator.compute-domain-dra-plugin.common.podSelector" }}
matchLabels:
  app: compute-domain-dra-plugin
  component: compute-domain-dra-plugin
{{- end }}

{{- define "fake-gpu-operator.compute-domain-dra-plugin.common.podTemplate.metadata" }}
annotations:
  checksum/topology: {{ include (print $.Template.BasePath "/topology-cm.yml") . | sha256sum }}
labels:
  app: compute-domain-dra-plugin
  component: compute-domain-dra-plugin
{{- end }}

{{- define "fake-gpu-operator.compute-domain-dra-plugin.common.podTemplate.spec" }}
containers:
  - image: "{{ .Values.computeDomainDraPlugin.image.repository }}:{{ .Values.computeDomainDraPlugin.image.tag | default .Chart.AppVersion }}"
    imagePullPolicy: "{{ .Values.computeDomainDraPlugin.image.pullPolicy }}"
    resources:
      {{- toYaml .Values.computeDomainDraPlugin.resources | nindent 12 }}
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
    name: compute-domain-dra-plugin-ctr
    securityContext:
      privileged: true
    terminationMessagePath: /dev/termination-log
    terminationMessagePolicy: File
    volumeMounts:
      - mountPath: /var/lib/kubelet/plugins_registry
        name: plugins-registry
      - mountPath: /var/lib/kubelet/plugins
        name: plugins
      - mountPath: /etc/cdi
        name: cdi
dnsPolicy: ClusterFirst
restartPolicy: Always
serviceAccountName: compute-domain-dra-plugin
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
{{- end }}
