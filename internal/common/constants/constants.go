package constants

const (
	AnnotationGpuIdx               = "runai-gpu"
	AnnotationGpuFraction          = "gpu-fraction"
	AnnotationPodGroupName         = "pod-group-name"
	AnnotationReservationPodGpuIdx = "run.ai/reserve_for_gpu_index"
	AnnotationMigMapping           = "run.ai/mig-mapping"
	AnnotationKwokNode             = "kwok.x-k8s.io/node"

	LabelGpuGroup                   = "runai-gpu-group"
	LabelGpuProduct                 = "nvidia.com/gpu.product"
	LabelMigConfigState             = "nvidia.com/mig.config.state"
	LabelFakeNodeDeploymentTemplate = "run.ai/fake-node-deployment-template"
	LabelTopologyCMNodeTopology     = "node-topology"
	LabelTopologyCMNodeName         = "node-name"

	ReservationNs = "runai-reservation"

	GpuResourceName = "nvidia.com/gpu"

	EnvFakeNode                     = "FAKE_NODE"
	EnvNodeName                     = "NODE_NAME"
	EnvTopologyCmName               = "TOPOLOGY_CM_NAME"
	EnvTopologyCmNamespace          = "TOPOLOGY_CM_NAMESPACE"
	EnvFakeGpuOperatorNs            = "FAKE_GPU_OPERATOR_NAMESPACE"
	EnvResourceReservationNamespace = "RESOURCE_RESERVATION_NAMESPACE"
	EnvPrometheusURL                = "PROMETHEUS_URL"
	EnvDisableNodeLabeling              = "DISABLE_NODE_LABELING"
	EnvRunaiIntegrationEnabled          = "RUNAI_INTEGRATION_ENABLED"
	EnvRunaiIntegrationPollingInterval  = "RUNAI_INTEGRATION_POLLING_INTERVAL"

	EnvComponentControllerEnabled = "COMPONENT_CONTROLLER_ENABLED"
	EnvDefaultImageRegistry       = "DEFAULT_IMAGE_REGISTRY"
	EnvFallbackImageTag           = "FALLBACK_IMAGE_TAG"
	EnvDraEnabled                 = "DRA_ENABLED"

	// Backend types
	BackendFake = "fake"
	BackendMock = "mock"

	// Managed resource labels
	LabelManagedBy      = "app.kubernetes.io/managed-by"
	LabelManagedByValue = "fake-gpu-operator"
	LabelComponent      = "fake-gpu-operator/component"
	LabelPool           = "fake-gpu-operator/pool"

	// Component names
	ComponentKwokDevicePlugin   = "kwok-gpu-device-plugin"
	ComponentKwokStatusExporter = "kwok-status-exporter"
	ComponentKwokDraPlugin      = "kwok-dra-plugin"
)
