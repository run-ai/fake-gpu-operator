/*
 * Copyright 2025 The Kubernetes Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package computedomaindraplugin

import (
	"context"
	"errors"
	"fmt"

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreclientset "k8s.io/client-go/kubernetes"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
	"k8s.io/dynamic-resource-allocation/resourceslice"
	"k8s.io/klog/v2"

	"github.com/run-ai/fake-gpu-operator/pkg/compute-domain/consts"
)

type computeDomainDriver struct {
	client      coreclientset.Interface
	helper      *kubeletplugin.Helper
	allocatable AllocatableComputeDomainDevices
	cancelCtx   func(error)
}

func NewComputeDomainDriver(ctx context.Context, config *Config) (*computeDomainDriver, error) {
	driver := &computeDomainDriver{
		client:    config.coreclient,
		cancelCtx: config.cancelMainCtx,
	}

	allocatable, err := enumerateComputeDomainDevices()
	if err != nil {
		return nil, fmt.Errorf("error enumerating ComputeDomain devices: %v", err)
	}
	driver.allocatable = allocatable

	helper, err := kubeletplugin.Start(
		ctx,
		driver,
		kubeletplugin.KubeClient(config.coreclient),
		kubeletplugin.NodeName(config.flags.nodeName),
		kubeletplugin.DriverName(consts.ComputeDomainDriverName),
		kubeletplugin.RegistrarDirectoryPath(config.flags.kubeletRegistrarDirectoryPath),
		kubeletplugin.PluginDataDirectoryPath(config.DriverPluginPath()),
	)
	if err != nil {
		return nil, err
	}
	driver.helper = helper

	devices := make([]resourceapi.Device, 0, 1)
	if device, exists := allocatable[deviceNameForChannel(0)]; exists {
		devices = append(devices, device)
	}
	resources := resourceslice.DriverResources{
		Pools: map[string]resourceslice.Pool{
			config.flags.nodeName: {
				Slices: []resourceslice.Slice{
					{
						Devices: devices,
					},
				},
			},
		},
	}

	if err := helper.PublishResources(ctx, resources); err != nil {
		return nil, err
	}

	klog.InfoS("ComputeDomain DRA plugin started", "nodeName", config.flags.nodeName)
	return driver, nil
}

func (d *computeDomainDriver) Shutdown(logger klog.Logger) error {
	d.helper.Stop()
	return nil
}

func (d *computeDomainDriver) PrepareResourceClaims(ctx context.Context, claims []*resourceapi.ResourceClaim) (map[types.UID]kubeletplugin.PrepareResult, error) {
	klog.Infof("PrepareResourceClaims is called: number of claims: %d", len(claims))
	result := make(map[types.UID]kubeletplugin.PrepareResult)

	for _, claim := range claims {
		result[claim.UID] = d.prepareResourceClaim(ctx, claim)
	}

	return result, nil
}

func (d *computeDomainDriver) prepareResourceClaim(_ context.Context, claim *resourceapi.ResourceClaim) kubeletplugin.PrepareResult {
	// Skeleton implementation - will be expanded in PR 2
	klog.Infof("PrepareResourceClaim called for claim %v (skeleton - not yet implemented)", claim.UID)
	return kubeletplugin.PrepareResult{
		Err: fmt.Errorf("PrepareResourceClaim not yet implemented"),
	}
}

func (d *computeDomainDriver) UnprepareResourceClaims(ctx context.Context, claims []kubeletplugin.NamespacedObject) (map[types.UID]error, error) {
	klog.Infof("UnprepareResourceClaims is called: number of claims: %d", len(claims))
	result := make(map[types.UID]error)

	for _, claim := range claims {
		result[claim.UID] = d.unprepareResourceClaim(ctx, claim)
	}

	return result, nil
}

func (d *computeDomainDriver) unprepareResourceClaim(_ context.Context, claim kubeletplugin.NamespacedObject) error {
	// Skeleton implementation - will be expanded in PR 2
	klog.Infof("UnprepareResourceClaim called for claim %v (skeleton - not yet implemented)", claim.UID)
	return nil
}

func (d *computeDomainDriver) HandleError(ctx context.Context, err error, msg string) {
	utilruntime.HandleErrorWithContext(ctx, err, msg)
	if !errors.Is(err, kubeletplugin.ErrRecoverable) && d.cancelCtx != nil {
		d.cancelCtx(fmt.Errorf("fatal background error: %w", err))
	}
}
