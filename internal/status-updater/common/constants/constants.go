package constants

const (
	// Pod annotations
	GpuIdxAnnotation       = "runai-gpu"
	GpuFractionAnnotation  = "gpu-fraction"
	PodGroupNameAnnotation = "pod-group-name"

	GpuGroupLabel = "runai-pod-group"

	ReservationNs = "runai-reservation"

	GpuResourceName = "nvidia.com/gpu"
)
