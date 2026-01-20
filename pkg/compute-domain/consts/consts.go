package consts

// ComputeDomain constants - matching NVIDIA's implementation
const (
	// ComputeDomainDriverName is the driver name for ComputeDomain resources
	// Same as NVIDIA's real implementation to ensure identical usage
	ComputeDomainDriverName = "compute-domain.nvidia.com"

	// ComputeDomainWorkloadDeviceClass is the DeviceClass for workload ResourceClaimTemplates
	ComputeDomainWorkloadDeviceClass = "compute-domain-default-channel.nvidia.com"

	// ComputeDomainFinalizer is the finalizer added to ComputeDomain CRs
	ComputeDomainFinalizer = "computedomain.resource.nvidia.com/finalizer"

	// ComputeDomainClaimLabel is the label used on ResourceClaims to identify which ComputeDomain they belong to
	ComputeDomainClaimLabel = "nvidia.com/computeDomain"

	// ComputeDomainTemplateLabel is the label used on ResourceClaimTemplates to identify which ComputeDomain they belong to
	ComputeDomainTemplateLabel = "resource.nvidia.com/computeDomain"

	// ComputeDomainTemplateTargetLabel is the label used on ResourceClaimTemplates to identify the target type
	ComputeDomainTemplateTargetLabel = "resource.nvidia.com/computeDomainTarget"
)
