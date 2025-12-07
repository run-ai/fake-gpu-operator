/*
 * Copyright 2023 The Kubernetes Authors.
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

package main

import (
	"fmt"
	"os"

	"k8s.io/klog/v2"
	"sigs.k8s.io/dra-example-driver/pkg/consts"

	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"
	cdiparser "tags.cncf.io/container-device-interface/pkg/parser"
	cdispec "tags.cncf.io/container-device-interface/specs-go"
)

const (
	cdiVendor = "k8s." + consts.DriverName
	cdiClass  = "gpu"
	cdiKind   = cdiVendor + "/" + cdiClass

	cdiCommonDeviceName = "common"
)

type CDIHandler struct {
	cache *cdiapi.Cache
}

func NewCDIHandler(config *Config) (*CDIHandler, error) {
	cache, err := cdiapi.NewCache(
		cdiapi.WithSpecDirs(config.flags.cdiRoot),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create a new CDI cache: %w", err)
	}
	handler := &CDIHandler{
		cache: cache,
	}

	return handler, nil
}

func (cdi *CDIHandler) CreateCommonSpecFile() error {
	klog.Info("Creating CDI common spec file")
	
	nodeName := os.Getenv("NODE_NAME")
	spec := &cdispec.Spec{
		Kind: cdiKind,
		Devices: []cdispec.Device{
			{
				Name: cdiCommonDeviceName,
				ContainerEdits: cdispec.ContainerEdits{
					Env: []string{
						fmt.Sprintf("KUBERNETES_NODE_NAME=%s", nodeName),
						fmt.Sprintf("DRA_RESOURCE_DRIVER_NAME=%s", consts.DriverName),
					},
					Mounts: []*cdispec.Mount{
						{
							HostPath:      "/var/lib/runai/bin/nvidia-smi",
							ContainerPath: "/bin/nvidia-smi",
							Options:       []string{"ro"},
						},
					},
				},
			},
		},
	}

	klog.Infof("CDI common spec: env vars: %v, mounts: hostPath=%s, containerPath=%s", 
		spec.Devices[0].ContainerEdits.Env,
		spec.Devices[0].ContainerEdits.Mounts[0].HostPath,
		spec.Devices[0].ContainerEdits.Mounts[0].ContainerPath)

	minVersion, err := cdiapi.MinimumRequiredVersion(spec)
	if err != nil {
		return fmt.Errorf("failed to get minimum required CDI spec version: %v", err)
	}
	spec.Version = minVersion

	specName, err := cdiapi.GenerateNameForTransientSpec(spec, cdiCommonDeviceName)
	if err != nil {
		return fmt.Errorf("failed to generate Spec name: %w", err)
	}

	klog.Infof("Writing CDI common spec file: %s", specName)
	err = cdi.cache.WriteSpec(spec, specName)
	if err != nil {
		klog.Errorf("Failed to write CDI common spec file: %v", err)
		return err
	}
	
	klog.Info("Successfully created CDI common spec file")
	return nil
}

func (cdi *CDIHandler) CreateClaimSpecFile(claimUID string, devices PreparedDevices, topologyJSON string) error {
	specName := cdiapi.GenerateTransientSpecName(cdiVendor, cdiClass, claimUID)
	klog.Infof("Creating CDI claim spec file for claim UID: %s, spec name: %s", claimUID, specName)

	spec := &cdispec.Spec{
		Kind:    cdiKind,
		Devices: []cdispec.Device{},
	}

	topologyInjected := false
	for _, device := range devices {
		envVars := []string{
			fmt.Sprintf("GPU_DEVICE_%s_RESOURCE_CLAIM=%s", device.DeviceName[4:], claimUID),
		}
		// Add topology JSON env var if provided
		if topologyJSON != "" {
			envVars = append(envVars, fmt.Sprintf("GPU_TOPOLOGY_JSON=%s", topologyJSON))
			topologyInjected = true
			klog.Infof("Injecting GPU_TOPOLOGY_JSON env var for device %s (length: %d chars)", device.DeviceName, len(topologyJSON))
		} else {
			klog.Warningf("No topology JSON provided for claim %s, skipping GPU_TOPOLOGY_JSON injection", claimUID)
		}

		claimEdits := cdiapi.ContainerEdits{
			ContainerEdits: &cdispec.ContainerEdits{
				Env: envVars,
			},
		}
		claimEdits.Append(device.ContainerEdits)

		cdiDevice := cdispec.Device{
			Name:           fmt.Sprintf("%s-%s", claimUID, device.DeviceName),
			ContainerEdits: *claimEdits.ContainerEdits,
		}

		klog.Infof("CDI device %s: env vars: %v", cdiDevice.Name, cdiDevice.ContainerEdits.Env)
		spec.Devices = append(spec.Devices, cdiDevice)
	}

	if topologyInjected {
		klog.Infof("Topology JSON injected into %d device(s) for claim %s", len(spec.Devices), claimUID)
	}

	minVersion, err := cdiapi.MinimumRequiredVersion(spec)
	if err != nil {
		klog.Errorf("Failed to get minimum required CDI spec version: %v", err)
		return fmt.Errorf("failed to get minimum required CDI spec version: %v", err)
	}
	spec.Version = minVersion

	klog.Infof("Writing CDI claim spec file: %s (version: %s)", specName, spec.Version)
	err = cdi.cache.WriteSpec(spec, specName)
	if err != nil {
		klog.Errorf("Failed to write CDI claim spec file: %v", err)
		return err
	}
	
	klog.Infof("Successfully created CDI claim spec file for claim %s with %d device(s)", claimUID, len(spec.Devices))
	return nil
}

func (cdi *CDIHandler) DeleteClaimSpecFile(claimUID string) error {
	specName := cdiapi.GenerateTransientSpecName(cdiVendor, cdiClass, claimUID)
	klog.Infof("Deleting CDI claim spec file for claim UID: %s, spec name: %s", claimUID, specName)
	err := cdi.cache.RemoveSpec(specName)
	if err != nil {
		klog.Errorf("Failed to delete CDI claim spec file: %v", err)
		return err
	}
	klog.Infof("Successfully deleted CDI claim spec file for claim %s", claimUID)
	return nil
}

func (cdi *CDIHandler) GetClaimDevices(claimUID string, devices []string) []string {
	cdiDevices := []string{
		cdiparser.QualifiedName(cdiVendor, cdiClass, cdiCommonDeviceName),
	}

	for _, device := range devices {
		cdiDevice := cdiparser.QualifiedName(cdiVendor, cdiClass, fmt.Sprintf("%s-%s", claimUID, device))
		cdiDevices = append(cdiDevices, cdiDevice)
	}

	return cdiDevices
}
