package constants

const (
	GpuIdxAnnotation               = "runai-gpu"
	GpuFractionAnnotation          = "gpu-fraction"
	PodGroupNameAnnotation         = "pod-group-name"
	ReservationPodGpuIdxAnnotation = "run.ai/reserve_for_gpu_index"

	GpuGroupLabel = "runai-pod-group"

	ReservationNs = "runai-reservation"

	GpuResourceName = "nvidia.com/gpu"
)
