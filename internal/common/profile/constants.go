package profile

const (
	// LabelGpuProfile is the label used to identify GPU profile ConfigMaps.
	LabelGpuProfile = "fake-gpu-operator/gpu-profile"

	// CmProfileKey is the data key inside profile ConfigMaps.
	CmProfileKey = "profile.yaml"

	// CmNamePrefix is the prefix for profile ConfigMap names.
	CmNamePrefix = "gpu-profile-"
)
