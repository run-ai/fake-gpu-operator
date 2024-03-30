package constants

const (
	GpuIdxAnnotation               = "runai-gpu"
	GpuFractionAnnotation          = "gpu-fraction"
	PodGroupNameAnnotation         = "pod-group-name"
	ReservationPodGpuIdxAnnotation = "run.ai/reserve_for_gpu_index"
	MigMappingAnnotation           = "run.ai/mig-mapping"
	KwokNodeAnnotation             = "kwok.x-k8s.io/node"

	GpuGroupLabel                   = "runai-gpu-group"
	GpuProductLabel                 = "nvidia.com/gpu.product"
	MigConfigStateLabel             = "nvidia.com/mig.config.state"
	FakeNodeDeploymentTemplateLabel = "run.ai/fake-node-deployment-template"

	ReservationNs = "runai-reservation"

	GpuResourceName = "nvidia.com/gpu"

	// GuyTodo: Use these constants in the code
	EnvFakeNode            = "FAKE_NODE"
	EnvNodeName            = "NODE_NAME"
	EnvTopologyCmName      = "TOPOLOGY_CM_NAME"
	EnvTopologyCmNamespace = "TOPOLOGY_CM_NAMESPACE"
	EnvFakeGpuOperatorNs   = "FAKE_GPU_OPERATOR_NAMESPACE"
)
