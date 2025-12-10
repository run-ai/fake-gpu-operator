package dra_plugin_gpu

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	resourceapi "k8s.io/api/resource/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreclientset "k8s.io/client-go/kubernetes"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/dynamic-resource-allocation/resourceslice"
	"k8s.io/klog/v2"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager"

	configapi "sigs.k8s.io/dra-example-driver/api/example.com/resource/gpu/v1alpha1"

	"github.com/run-ai/fake-gpu-operator/internal/common/topology"
	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdispec "tags.cncf.io/container-device-interface/specs-go"
)

type AllocatableDevices map[string]resourceapi.Device
type PreparedDevices []*PreparedDevice
type PreparedClaims map[string]PreparedDevices
type PerDeviceCDIContainerEdits map[string]*cdiapi.ContainerEdits

type OpaqueDeviceConfig struct {
	Requests []string
	Config   runtime.Object
}

type PreparedDevice struct {
	drapbv1.Device
	ContainerEdits *cdiapi.ContainerEdits
}

func (pds PreparedDevices) GetDevices() []*drapbv1.Device {
	if pds == nil {
		return nil
	}
	if len(pds) == 0 {
		return []*drapbv1.Device{}
	}
	var devices []*drapbv1.Device
	for _, pd := range pds {
		devices = append(devices, &pd.Device)
	}
	return devices
}

type DeviceState struct {
	sync.Mutex
	cdi               *CDIHandler
	allocatable       AllocatableDevices
	checkpointManager checkpointmanager.CheckpointManager
	nodeName          string
	coreclient        coreclientset.Interface
	helper            *kubeletplugin.Helper
}

// waitForGPUAnnotation retries enumerating devices with exponential backoff
// until the GPU annotation is found on the node.
func waitForGPUAnnotation(ctx context.Context, coreclient coreclientset.Interface, nodeName string) (AllocatableDevices, error) {
	logger := klog.FromContext(ctx)

	backoff := wait.Backoff{
		Duration: 2 * time.Second, // Initial delay
		Factor:   2.0,             // Multiply by 2 each time
		Steps:    10,              // Maximum 10 retries
		Cap:      5 * time.Minute, // Cap at 5 minutes
	}

	var allocatable AllocatableDevices
	err := wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		var err error
		allocatable, err = enumerateAllPossibleDevices(ctx, coreclient, nodeName)
		if err != nil {
			// Check if it's an annotation missing error (retryable)
			if strings.Contains(err.Error(), "annotation") &&
				(strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "empty")) {
				logger.V(2).Info("Waiting for GPU annotation", "node", nodeName, "error", err)
				return false, nil // Retry
			}
			// Other errors are not retryable
			return false, err
		}
		// Success
		logger.Info("Successfully found GPU annotation", "deviceCount", len(allocatable))
		return true, nil
	})

	if err != nil {
		return nil, fmt.Errorf("error enumerating all possible devices after retries: %w", err)
	}

	return allocatable, nil
}

func NewDeviceState(ctx context.Context, config *Config, helper *kubeletplugin.Helper) (*DeviceState, error) {
	allocatable, err := waitForGPUAnnotation(ctx, config.CoreClient, config.Flags.NodeName)
	if err != nil {
		return nil, fmt.Errorf("error enumerating all possible devices: %v", err)
	}

	cdi, err := NewCDIHandler(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create CDI handler: %v", err)
	}

	err = cdi.CreateCommonSpecFile()
	if err != nil {
		return nil, fmt.Errorf("unable to create CDI spec file for common edits: %v", err)
	}

	checkpointManager, err := checkpointmanager.NewCheckpointManager(config.DriverPluginPath())
	if err != nil {
		return nil, fmt.Errorf("unable to create checkpoint manager: %v", err)
	}

	state := &DeviceState{
		cdi:               cdi,
		allocatable:       allocatable,
		checkpointManager: checkpointManager,
		nodeName:          config.Flags.NodeName,
		coreclient:        config.CoreClient,
		helper:            helper,
	}

	checkpoints, err := state.checkpointManager.ListCheckpoints()
	if err != nil {
		return nil, fmt.Errorf("unable to list checkpoints: %v", err)
	}

	for _, c := range checkpoints {
		if c == DriverPluginCheckpointFile {
			return state, nil
		}
	}

	checkpoint := newCheckpoint()
	if err := state.checkpointManager.CreateCheckpoint(DriverPluginCheckpointFile, checkpoint); err != nil {
		return nil, fmt.Errorf("unable to sync to checkpoint: %v", err)
	}

	return state, nil
}

func (s *DeviceState) Prepare(ctx context.Context, claim *resourceapi.ResourceClaim) ([]*drapbv1.Device, error) {
	s.Lock()
	defer s.Unlock()

	claimUID := string(claim.UID)

	checkpoint := newCheckpoint()
	if err := s.checkpointManager.GetCheckpoint(DriverPluginCheckpointFile, checkpoint); err != nil {
		return nil, fmt.Errorf("unable to sync from checkpoint: %v", err)
	}
	preparedClaims := checkpoint.V1.PreparedClaims

	if preparedClaims[claimUID] != nil {
		return preparedClaims[claimUID].GetDevices(), nil
	}

	preparedDevices, err := s.prepareDevices(claim)
	if err != nil {
		return nil, fmt.Errorf("prepare failed: %v", err)
	}

	// Get topology JSON from node annotation
	topologyJSON, err := s.getTopologyJSON(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to get topology JSON: %v", err)
	}

	if err = s.cdi.CreateClaimSpecFile(claimUID, preparedDevices, topologyJSON); err != nil {
		return nil, fmt.Errorf("unable to create CDI spec file for claim: %v", err)
	}

	preparedClaims[claimUID] = preparedDevices
	if err := s.checkpointManager.CreateCheckpoint(DriverPluginCheckpointFile, checkpoint); err != nil {
		return nil, fmt.Errorf("unable to sync to checkpoint: %v", err)
	}

	return preparedClaims[claimUID].GetDevices(), nil
}

func (s *DeviceState) Unprepare(claimUID string) error {
	s.Lock()
	defer s.Unlock()

	checkpoint := newCheckpoint()
	if err := s.checkpointManager.GetCheckpoint(DriverPluginCheckpointFile, checkpoint); err != nil {
		return fmt.Errorf("unable to sync from checkpoint: %v", err)
	}
	preparedClaims := checkpoint.V1.PreparedClaims

	if preparedClaims[claimUID] == nil {
		return nil
	}

	if err := s.unprepareDevices(claimUID, preparedClaims[claimUID]); err != nil {
		return fmt.Errorf("unprepare failed: %v", err)
	}

	err := s.cdi.DeleteClaimSpecFile(claimUID)
	if err != nil {
		return fmt.Errorf("unable to delete CDI spec file for claim: %v", err)
	}

	delete(preparedClaims, claimUID)
	if err := s.checkpointManager.CreateCheckpoint(DriverPluginCheckpointFile, checkpoint); err != nil {
		return fmt.Errorf("unable to sync to checkpoint: %v", err)
	}

	return nil
}

func (s *DeviceState) prepareDevices(claim *resourceapi.ResourceClaim) (PreparedDevices, error) {
	if claim.Status.Allocation == nil {
		return nil, fmt.Errorf("claim not yet allocated")
	}

	// Retrieve the full set of device configs for the driver.
	configs, err := GetOpaqueDeviceConfigs(
		configapi.Decoder,
		DriverName,
		claim.Status.Allocation.Devices.Config,
	)
	if err != nil {
		return nil, fmt.Errorf("error getting opaque device configs: %v", err)
	}

	// Add the default GPU Config to the front of the config list with the
	// lowest precedence. This guarantees there will be at least one config in
	// the list with len(Requests) == 0 for the lookup below.
	configs = slices.Insert(configs, 0, &OpaqueDeviceConfig{
		Requests: []string{},
		Config:   configapi.DefaultGpuConfig(),
	})

	// Look through the configs and figure out which one will be applied to
	// each device allocation result based on their order of precedence.
	configResultsMap := make(map[runtime.Object][]*resourceapi.DeviceRequestAllocationResult)
	for _, result := range claim.Status.Allocation.Devices.Results {
		if _, exists := s.allocatable[result.Device]; !exists {
			return nil, fmt.Errorf("requested GPU is not allocatable: %v", result.Device)
		}
		for _, c := range slices.Backward(configs) {
			if len(c.Requests) == 0 || slices.Contains(c.Requests, result.Request) {
				configResultsMap[c.Config] = append(configResultsMap[c.Config], &result)
				break
			}
		}
	}

	// Normalize, validate, and apply all configs associated with devices that
	// need to be prepared. Track container edits generated from applying the
	// config to the set of device allocation results.
	perDeviceCDIContainerEdits := make(PerDeviceCDIContainerEdits)
	for c, results := range configResultsMap {
		// Cast the opaque config to a GpuConfig
		var config *configapi.GpuConfig
		switch castConfig := c.(type) {
		case *configapi.GpuConfig:
			config = castConfig
		default:
			return nil, fmt.Errorf("runtime object is not a regognized configuration")
		}

		// Normalize the config to set any implied defaults.
		if err := config.Normalize(); err != nil {
			return nil, fmt.Errorf("error normalizing GPU config: %w", err)
		}

		// Validate the config to ensure its integrity.
		if err := config.Validate(); err != nil {
			return nil, fmt.Errorf("error validating GPU config: %w", err)
		}

		// Apply the config to the list of results associated with it.
		containerEdits, err := s.applyConfig(config, results)
		if err != nil {
			return nil, fmt.Errorf("error applying GPU config: %w", err)
		}

		// Merge any new container edits with the overall per device map.
		for k, v := range containerEdits {
			perDeviceCDIContainerEdits[k] = v
		}
	}

	// Walk through each config and its associated device allocation results
	// and construct the list of prepared devices to return.
	var preparedDevices PreparedDevices
	for _, results := range configResultsMap {
		for _, result := range results {
			device := &PreparedDevice{
				Device: drapbv1.Device{
					RequestNames: []string{result.Request},
					PoolName:     result.Pool,
					DeviceName:   result.Device,
					CDIDeviceIDs: s.cdi.GetClaimDevices(string(claim.UID), []string{result.Device}),
				},
				ContainerEdits: perDeviceCDIContainerEdits[result.Device],
			}
			preparedDevices = append(preparedDevices, device)
		}
	}

	return preparedDevices, nil
}

// getTopologyJSON retrieves topology from node annotation and converts it to JSON
func (s *DeviceState) getTopologyJSON(ctx context.Context) (string, error) {
	klog.Info("Retrieving topology JSON from node annotation")

	// Fetch the node
	node, err := s.coreclient.CoreV1().Nodes().Get(ctx, s.nodeName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Failed to get node %s: %v", s.nodeName, err)
		return "", fmt.Errorf("failed to get node %s: %w", s.nodeName, err)
	}

	// Read the annotation
	annotationValue, exists := node.Annotations[AnnotationGpuFakeDevices]
	if !exists {
		klog.Warningf("Annotation %s not found on node %s", AnnotationGpuFakeDevices, s.nodeName)
		return "", fmt.Errorf("annotation %s not found on node %s", AnnotationGpuFakeDevices, s.nodeName)
	}

	if annotationValue == "" {
		klog.Warningf("Annotation %s is empty on node %s", AnnotationGpuFakeDevices, s.nodeName)
		return "", fmt.Errorf("annotation %s is empty on node %s", AnnotationGpuFakeDevices, s.nodeName)
	}

	klog.Infof("Found annotation %s on node %s (length: %d chars)", AnnotationGpuFakeDevices, s.nodeName, len(annotationValue))

	// Parse JSON as NodeTopology
	var nodeTopology topology.NodeTopology
	if err := json.Unmarshal([]byte(annotationValue), &nodeTopology); err != nil {
		klog.Errorf("Failed to parse annotation %s: %v", AnnotationGpuFakeDevices, err)
		return "", fmt.Errorf("failed to parse annotation %s: %w", AnnotationGpuFakeDevices, err)
	}

	klog.Infof("Parsed annotation: gpuCount=%d, gpuProduct=%s, gpuMemory=%d", len(nodeTopology.Gpus), nodeTopology.GpuProduct, nodeTopology.GpuMemory)

	// Serialize to JSON
	topologyJSON, err := json.Marshal(nodeTopology)
	if err != nil {
		klog.Errorf("Failed to marshal topology: %v", err)
		return "", fmt.Errorf("failed to marshal topology: %w", err)
	}

	klog.Infof("Successfully generated topology JSON (length: %d chars) for node %s", len(topologyJSON), s.nodeName)
	return string(topologyJSON), nil
}

func (s *DeviceState) unprepareDevices(claimUID string, devices PreparedDevices) error {
	return nil
}

// sanitizeDeviceNameForEnvVar replaces hyphens with underscores in device names
// to make them valid shell environment variable names.
func sanitizeDeviceNameForEnvVar(deviceName string) string {
	return strings.ReplaceAll(deviceName, "-", "_")
}

// applyConfig applies a configuration to a set of device allocation results.
//
// In this example driver there is no actual configuration applied. We simply
// define a set of environment variables to be injected into the containers
// that include a given device. A real driver would likely need to do some sort
// of hardware configuration as well, based on the config passed in.
func (s *DeviceState) applyConfig(config *configapi.GpuConfig, results []*resourceapi.DeviceRequestAllocationResult) (PerDeviceCDIContainerEdits, error) {
	perDeviceEdits := make(PerDeviceCDIContainerEdits)

	for _, result := range results {
		// Device name is now just the UUID (lowercase), so use it directly
		deviceID := sanitizeDeviceNameForEnvVar(result.Device)
		envs := []string{
			fmt.Sprintf("GPU_DEVICE_%s=%s", deviceID, result.Device),
		}

		if config.Sharing != nil {
			envs = append(envs, fmt.Sprintf("GPU_DEVICE_%s_SHARING_STRATEGY=%s", deviceID, config.Sharing.Strategy))
		}

		switch {
		case config.Sharing.IsTimeSlicing():
			tsconfig, err := config.Sharing.GetTimeSlicingConfig()
			if err != nil {
				return nil, fmt.Errorf("unable to get time slicing config for device %v: %w", result.Device, err)
			}
			envs = append(envs, fmt.Sprintf("GPU_DEVICE_%s_TIMESLICE_INTERVAL=%v", deviceID, tsconfig.Interval))
		case config.Sharing.IsSpacePartitioning():
			spconfig, err := config.Sharing.GetSpacePartitioningConfig()
			if err != nil {
				return nil, fmt.Errorf("unable to get space partitioning config for device %v: %w", result.Device, err)
			}
			envs = append(envs, fmt.Sprintf("GPU_DEVICE_%s_PARTITION_COUNT=%v", deviceID, spconfig.PartitionCount))
		}

		edits := &cdispec.ContainerEdits{
			Env: envs,
		}

		perDeviceEdits[result.Device] = &cdiapi.ContainerEdits{ContainerEdits: edits}
	}

	return perDeviceEdits, nil
}

// GetOpaqueDeviceConfigs returns an ordered list of the configs contained in possibleConfigs for this driver.
//
// Configs can either come from the resource claim itself or from the device
// class associated with the request. Configs coming directly from the resource
// claim take precedence over configs coming from the device class. Moreover,
// configs found later in the list of configs attached to its source take
// precedence over configs found earlier in the list for that source.
//
// All of the configs relevant to the driver from the list of possibleConfigs
// will be returned in order of precedence (from lowest to highest). If no
// configs are found, nil is returned.
func GetOpaqueDeviceConfigs(
	decoder runtime.Decoder,
	driverName string,
	possibleConfigs []resourceapi.DeviceAllocationConfiguration,
) ([]*OpaqueDeviceConfig, error) {
	// Collect all configs in order of reverse precedence.
	var classConfigs []resourceapi.DeviceAllocationConfiguration
	var claimConfigs []resourceapi.DeviceAllocationConfiguration
	var candidateConfigs []resourceapi.DeviceAllocationConfiguration
	for _, config := range possibleConfigs {
		switch config.Source {
		case resourceapi.AllocationConfigSourceClass:
			classConfigs = append(classConfigs, config)
		case resourceapi.AllocationConfigSourceClaim:
			claimConfigs = append(claimConfigs, config)
		default:
			return nil, fmt.Errorf("invalid config source: %v", config.Source)
		}
	}
	candidateConfigs = append(candidateConfigs, classConfigs...)
	candidateConfigs = append(candidateConfigs, claimConfigs...)

	// Decode all configs that are relevant for the driver.
	var resultConfigs []*OpaqueDeviceConfig
	for _, config := range candidateConfigs {
		// If this is nil, the driver doesn't support some future API extension
		// and needs to be updated.
		if config.Opaque == nil {
			return nil, fmt.Errorf("only opaque parameters are supported by this driver")
		}

		// Configs for different drivers may have been specified because a
		// single request can be satisfied by different drivers. This is not
		// an error -- drivers must skip over other driver's configs in order
		// to support this.
		if config.Opaque.Driver != driverName {
			continue
		}

		decodedConfig, err := runtime.Decode(decoder, config.Opaque.Parameters.Raw)
		if err != nil {
			return nil, fmt.Errorf("error decoding config parameters: %w", err)
		}

		resultConfig := &OpaqueDeviceConfig{
			Requests: config.Requests,
			Config:   decodedConfig,
		}

		resultConfigs = append(resultConfigs, resultConfig)
	}

	return resultConfigs, nil
}

// UpdateDevicesFromAnnotation updates the allocatable devices from the node annotation
// and re-publishes resources. This method is thread-safe.
func (s *DeviceState) UpdateDevicesFromAnnotation(ctx context.Context) error {
	s.Lock()
	defer s.Unlock()

	allocatable, err := enumerateAllPossibleDevices(ctx, s.coreclient, s.nodeName)
	if err != nil {
		return fmt.Errorf("failed to enumerate devices from annotation: %w", err)
	}

	s.allocatable = allocatable

	// Re-publish resources with updated devices
	devices := make([]resourceapi.Device, 0, len(s.allocatable))
	for device := range maps.Values(s.allocatable) {
		devices = append(devices, device)
	}
	resources := resourceslice.DriverResources{
		Pools: map[string]resourceslice.Pool{
			s.nodeName: {
				Slices: []resourceslice.Slice{
					{
						Devices: devices,
					},
				},
			},
		},
	}

	if s.helper != nil {
		if err := s.helper.PublishResources(ctx, resources); err != nil {
			return fmt.Errorf("failed to publish updated resources: %w", err)
		}
	}

	klog.FromContext(ctx).Info("Successfully updated devices from annotation", "deviceCount", len(devices))
	return nil
}
