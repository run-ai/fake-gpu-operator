package constants

const (
	AnnotationGpuIdx               = "runai-gpu"
	AnnotationGpuFraction          = "gpu-fraction"
	AnnotationPodGroupName         = "pod-group-name"
	AnnotationReservationPodGpuIdx = "run.ai/reserve_for_gpu_index"
	AnnotationMigMapping           = "run.ai/mig-mapping"
	AnnotationKwokNode             = "kwok.x-k8s.io/node"

	// Label Keys
	LabelGpuGroup                   = "runai-gpu-group"
	LabelGpuProduct                 = "nvidia.com/gpu.product"
	LabelMigConfigState             = "nvidia.com/mig.config.state"
	LabelFakeNodeDeployment         = "run.ai/fake-node-deployment"
	LabelFakeNodeDeploymentTemplate = "run.ai/fake-node-deployment-template"
	LabelTopologyCMNodeTopology     = "node-topology"
	LabelTopologyCMNodeName         = "node-name"
	LabelApp                        = "app"

	DCGMExporterApp     = "nvidia-dcgm-exporter"
	KwokDCGMExporterApp = "kwok-nvidia-dcgm-exporter"

	ReservationNs = "runai-reservation"

	GpuResourceName = "nvidia.com/gpu"

	EnvFakeNode                         = "FAKE_NODE"
	EnvNodeName                         = "NODE_NAME"
	EnvTopologyCmName                   = "TOPOLOGY_CM_NAME"
	EnvTopologyCmNamespace              = "TOPOLOGY_CM_NAMESPACE"
	EnvFakeGpuOperatorNs                = "FAKE_GPU_OPERATOR_NAMESPACE"
	EnvImpersonatePodName               = "IMPERSONATE_POD_NAME"
	EnvImpersonatePodIP                 = "IMPERSONATE_IP"
	EnvExportPrometheusLabelEnrichments = "EXPORT_PROMETHEUS_LABEL_ENRICHMENTS"
)
