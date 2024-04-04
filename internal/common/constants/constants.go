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

	EnvFakeNode            = "FAKE_NODE"
	EnvNodeName            = "NODE_NAME"
	EnvTopologyCmName      = "TOPOLOGY_CM_NAME"
	EnvTopologyCmNamespace = "TOPOLOGY_CM_NAMESPACE"
	EnvFakeGpuOperatorNs   = "FAKE_GPU_OPERATOR_NAMESPACE"
)
