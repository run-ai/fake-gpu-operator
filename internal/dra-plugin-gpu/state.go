package dra_plugin_gpu

import (
	"context"
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/runtime"
	coreclientset "k8s.io/client-go/kubernetes"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/dynamic-resource-allocation/resourceslice"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"

	configapi "sigs.k8s.io/dra-example-driver/api/example.com/resource/gpu/v1alpha1"

	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdispec "tags.cncf.io/container-device-interface/specs-go"
)

type AllocatableDevices map[string]resourceapi.Device
type PreparedDevices []*PreparedDevice
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
	cdi         *CDIHandler
	allocatable AllocatableDevices
	nodeName    string
	coreclient  coreclientset.Interface
	helper      *kubeletplugin.Helper
}

// waitForTopology polls for the topology from the HTTP server every 3 seconds until available.
func waitForTopology(ctx context.Context, nodeName string) (AllocatableDevices, error) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		allocatable, err := enumerateAllPossibleDevices(nodeName)
		if err == nil {
			log.Printf("Successfully fetched topology from server, deviceCount=%d", len(allocatable))
			return allocatable, nil
		}

		log.Printf("Waiting for topology server for node %s: %v", nodeName, err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func NewDeviceState(ctx context.Context, config *Config, helper *kubeletplugin.Helper) (*DeviceState, error) {
	allocatable, err := waitForTopology(ctx, config.Flags.NodeName)
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

	return &DeviceState{
		cdi:         cdi,
		allocatable: allocatable,
		nodeName:    config.Flags.NodeName,
		coreclient:  config.CoreClient,
		helper:      helper,
	}, nil
}

func (s *DeviceState) Prepare(ctx context.Context, claim *resourceapi.ResourceClaim) ([]*drapbv1.Device, error) {
	s.Lock()
	defer s.Unlock()

	claimUID := string(claim.UID)

	preparedDevices, err := s.prepareDevices(claim)
	if err != nil {
		return nil, fmt.Errorf("prepare failed: %v", err)
	}

	// CDI file creation is idempotent (overwrites if exists)
	if err = s.cdi.CreateClaimSpecFile(claimUID, preparedDevices); err != nil {
		return nil, fmt.Errorf("unable to create CDI spec file for claim: %v", err)
	}

	return preparedDevices.GetDevices(), nil
}

func (s *DeviceState) Unprepare(claimUID string) error {
	s.Lock()
	defer s.Unlock()

	// CDI file deletion is idempotent (handles missing files gracefully)
	return s.cdi.DeleteClaimSpecFile(claimUID)
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

// GetOpaqueDeviceConfigs returns configs from possibleConfigs that match this driver.
func GetOpaqueDeviceConfigs(
	decoder runtime.Decoder,
	driverName string,
	possibleConfigs []resourceapi.DeviceAllocationConfiguration,
) ([]*OpaqueDeviceConfig, error) {
	var resultConfigs []*OpaqueDeviceConfig
	for _, config := range possibleConfigs {
		if config.Opaque == nil || config.Opaque.Driver != driverName {
			continue
		}

		decodedConfig, err := runtime.Decode(decoder, config.Opaque.Parameters.Raw)
		if err != nil {
			return nil, fmt.Errorf("error decoding config: %w", err)
		}

		resultConfigs = append(resultConfigs, &OpaqueDeviceConfig{
			Requests: config.Requests,
			Config:   decodedConfig,
		})
	}
	return resultConfigs, nil
}

// UpdateDevicesFromTopology updates the allocatable devices from the topology server
// and re-publishes resources. This method is thread-safe.
func (s *DeviceState) UpdateDevicesFromTopology(ctx context.Context) error {
	s.Lock()
	defer s.Unlock()

	allocatable, err := enumerateAllPossibleDevices(s.nodeName)
	if err != nil {
		return fmt.Errorf("failed to enumerate devices from topology server: %w", err)
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

	log.Printf("Successfully updated devices from annotation, deviceCount=%d", len(devices))
	return nil
}
