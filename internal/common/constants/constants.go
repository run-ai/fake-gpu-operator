package constants

const (
	GpuIdxAnnotation               = "runai-gpu"
	GpuFractionAnnotation          = "gpu-fraction"
	PodGroupNameAnnotation         = "pod-group-name"
	ReservationPodGpuIdxAnnotation = "run.ai/reserve_for_gpu_index"
	MigMappingAnnotation           = "run.ai/mig-mapping"

	GpuGroupLabel       = "runai-gpu-group"
	GpuProductLabel     = "nvidia.com/gpu.product"
	MigConfigStateLabel = "nvidia.com/mig.config.state"

	ReservationNs = "runai-reservation"

	GpuResourceName = "nvidia.com/gpu"

	// GuyTodo: Replace all references to this
	EnvFakeNode            = "FAKE_NODE"
	EnvNodeName            = "NODE_NAME"
	EnvTopologyCmName      = "TOPOLOGY_CM_NAME"
	EnvTopologyCmNamespace = "TOPOLOGY_CM_NAMESPACE"
)
