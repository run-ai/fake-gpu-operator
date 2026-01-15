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
	"fmt"
	"os"
	"path/filepath"

	"github.com/run-ai/fake-gpu-operator/pkg/compute-domain/consts"

	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdiparser "tags.cncf.io/container-device-interface/pkg/parser"
	cdispec "tags.cncf.io/container-device-interface/specs-go"
)

const (
	computeDomainCDIVendor = "k8s." + consts.ComputeDomainDriverName
	computeDomainCDIClass  = "computedomain"
	computeDomainCDIKind   = computeDomainCDIVendor + "/" + computeDomainCDIClass

	computeDomainCDICommonDeviceName = "common"
)

type ComputeDomainCDIHandler struct {
	cache        *cdiapi.Cache
	nvcdiDevice  *computeDomainNvcdiDevice
	deviceRoot   string
	claimDevName string
}

func NewComputeDomainCDIHandler(config *Config) (*ComputeDomainCDIHandler, error) {
	cache, err := cdiapi.NewCache(
		cdiapi.WithSpecDirs(config.flags.cdiRoot),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create a new CDI cache: %w", err)
	}
	deviceRoot := filepath.Join(config.DriverPluginPath(), "nvcdi")
	nvcdiDevice, err := newComputeDomainNvcdiDevice(deviceRoot)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize nvcdi device helper: %w", err)
	}

	handler := &ComputeDomainCDIHandler{
		cache:        cache,
		nvcdiDevice:  nvcdiDevice,
		deviceRoot:   deviceRoot,
		claimDevName: "channel",
	}

	return handler, nil
}

func (cdi *ComputeDomainCDIHandler) CreateCommonSpecFile() error {
	spec := &cdispec.Spec{
		Kind: computeDomainCDIKind,
		Devices: []cdispec.Device{
			{
				Name: computeDomainCDICommonDeviceName,
				ContainerEdits: cdispec.ContainerEdits{
					Env: []string{
						fmt.Sprintf("KUBERNETES_NODE_NAME=%s", os.Getenv("NODE_NAME")),
						fmt.Sprintf("DRA_RESOURCE_DRIVER_NAME=%s", consts.ComputeDomainDriverName),
					},
				},
			},
		},
	}

	minVersion, err := cdispec.MinimumRequiredVersion(spec)
	if err != nil {
		return fmt.Errorf("failed to get minimum required CDI spec version: %v", err)
	}
	spec.Version = minVersion

	specName, err := cdiapi.GenerateNameForTransientSpec(spec, computeDomainCDICommonDeviceName)
	if err != nil {
		return fmt.Errorf("failed to generate Spec name: %w", err)
	}

	return cdi.cache.WriteSpec(spec, specName)
}

func (cdi *ComputeDomainCDIHandler) CreateClaimSpecFile(claimUID string, devices ComputeDomainPreparedDevices) error {
	specName := cdiapi.GenerateTransientSpecName(computeDomainCDIVendor, computeDomainCDIClass, claimUID)

	spec := &cdispec.Spec{
		Kind:    computeDomainCDIKind,
		Devices: []cdispec.Device{},
	}

	for _, device := range devices {
		claimEdits := cdiapi.ContainerEdits{
			ContainerEdits: &cdispec.ContainerEdits{
				Env: []string{
					fmt.Sprintf("COMPUTE_DOMAIN_DEVICE_%s_RESOURCE_CLAIM=%s", device.DeviceName, claimUID),
				},
			},
		}
		if device.ContainerEdits != nil {
			claimEdits.Append(device.ContainerEdits)
		}

		cdiDevice := cdispec.Device{
			Name:           fmt.Sprintf("%s-%s", claimUID, device.DeviceName),
			ContainerEdits: *claimEdits.ContainerEdits,
		}

		spec.Devices = append(spec.Devices, cdiDevice)
	}

	minVersion, err := cdiapi.MinimumRequiredVersion(spec)
	if err != nil {
		return fmt.Errorf("failed to get minimum required CDI spec version: %v", err)
	}
	spec.Version = minVersion

	return cdi.cache.WriteSpec(spec, specName)
}

func (cdi *ComputeDomainCDIHandler) DeleteClaimSpecFile(claimUID string) error {
	specName := cdiapi.GenerateTransientSpecName(computeDomainCDIVendor, computeDomainCDIClass, claimUID)
	return cdi.cache.RemoveSpec(specName)
}

func (cdi *ComputeDomainCDIHandler) GetClaimDevices(claimUID string, devices []string) []string {
	cdiDevices := make([]string, 0, len(devices))
	for _, device := range devices {
		cdiDevice := cdiparser.QualifiedName(computeDomainCDIVendor, computeDomainCDIClass, fmt.Sprintf("%s-%s", claimUID, device))
		cdiDevices = append(cdiDevices, cdiDevice)
	}
	return cdiDevices
}

func (cdi *ComputeDomainCDIHandler) CreateDomainCDIDevice(domainInfo *DomainInfo) (*cdiapi.ContainerEdits, error) {
	if domainInfo == nil {
		return nil, fmt.Errorf("domain info is required to create CDI device")
	}
	return cdi.nvcdiDevice.ContainerEdits(domainInfo)
}
