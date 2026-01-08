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
	"encoding/json"
	"fmt"
	"sync"
	"time"

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager"

	cdiapi "tags.cncf.io/container-device-interface/pkg/cdi"

	"github.com/run-ai/fake-gpu-operator/pkg/compute-domain/consts"
)

type ComputeDomainPreparedDevices []*ComputeDomainPreparedDevice
type ComputeDomainPreparedClaims map[string]ComputeDomainPreparedDevices

type ComputeDomainPreparedDevice struct {
	drapbv1.Device
	ContainerEdits *cdiapi.ContainerEdits
}

func (pds ComputeDomainPreparedDevices) GetDevices() []*drapbv1.Device {
	var devices []*drapbv1.Device
	for _, pd := range pds {
		devices = append(devices, &pd.Device)
	}
	return devices
}

type DomainInfo struct {
	DomainID          string
	ComputeDomainName string
	ComputeDomainUID  types.UID
	Nodes             []string
	Pods              []types.NamespacedName
	Claims            []string // ResourceClaim UIDs
	CreatedAt         time.Time
}

type ComputeDomainState struct {
	sync.Mutex
	cdi               *ComputeDomainCDIHandler
	allocatable       AllocatableComputeDomainDevices
	checkpointManager checkpointmanager.CheckpointManager
	domains           map[string]*DomainInfo // domainID -> DomainInfo
	nodeName          string
}

func NewComputeDomainState(config *Config) (*ComputeDomainState, error) {
	allocatable, err := enumerateComputeDomainDevices()
	if err != nil {
		return nil, fmt.Errorf("error enumerating ComputeDomain devices: %v", err)
	}

	cdi, err := NewComputeDomainCDIHandler(config)
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

	state := &ComputeDomainState{
		cdi:               cdi,
		allocatable:       allocatable,
		checkpointManager: checkpointManager,
		domains:           make(map[string]*DomainInfo),
		nodeName:          config.flags.nodeName,
	}

	checkpoints, err := state.checkpointManager.ListCheckpoints()
	if err != nil {
		return nil, fmt.Errorf("unable to list checkpoints: %v", err)
	}

	for _, c := range checkpoints {
		if c == DriverPluginCheckpointFile {
			// Load checkpoint
			checkpoint := newComputeDomainCheckpoint()
			err := state.checkpointManager.GetCheckpoint(DriverPluginCheckpointFile, checkpoint)
			if err != nil {
				return nil, fmt.Errorf("unable to get checkpoint: %v", err)
			}
			if err := checkpoint.VerifyChecksum(); err != nil {
				return nil, fmt.Errorf("checkpoint checksum verification failed: %v", err)
			}
			if checkpoint.V1 != nil {
				state.domains = checkpoint.V1.Domains
			}
			return state, nil
		}
	}

	checkpoint := newComputeDomainCheckpoint()
	err = state.checkpointManager.CreateCheckpoint(DriverPluginCheckpointFile, checkpoint)
	if err != nil {
		return nil, fmt.Errorf("unable to create checkpoint: %v", err)
	}

	return state, nil
}

func (s *ComputeDomainState) Prepare(claim *resourceapi.ResourceClaim) (ComputeDomainPreparedDevices, error) {
	s.Lock()
	defer s.Unlock()

	claimUID := string(claim.UID)

	checkpoint := newComputeDomainCheckpoint()
	if err := s.checkpointManager.GetCheckpoint(DriverPluginCheckpointFile, checkpoint); err != nil {
		return nil, fmt.Errorf("unable to sync from checkpoint: %v", err)
	}

	if checkpoint.V1 == nil {
		checkpoint.V1 = &ComputeDomainCheckpointV1{
			PreparedClaims: make(ComputeDomainPreparedClaims),
			Domains:        make(map[string]*DomainInfo),
		}
	}

	preparedClaims := checkpoint.V1.PreparedClaims
	domains := checkpoint.V1.Domains

	if preparedClaims[claimUID] != nil {
		if err := s.ensureClaimCDIArtifacts(claimUID, preparedClaims[claimUID]); err != nil {
			return nil, err
		}
		return preparedClaims[claimUID], nil
	}

	deviceClassName := s.getDeviceClassName(claim)
	if deviceClassName == "" {
		return nil, fmt.Errorf("unable to determine device class from claim")
	}

	computeDomainID := s.extractComputeDomainID(claim)
	if computeDomainID == "" {
		return nil, fmt.Errorf("unable to extract ComputeDomain ID from claim")
	}

	domainID := s.getOrCreateDomain(domains, computeDomainID, claim)

	domainInfo := domains[domainID]
	if domainInfo == nil {
		return nil, fmt.Errorf("domain %s not found", domainID)
	}

	cdiEdits, err := s.cdi.CreateDomainCDIDevice(domainInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to create CDI device: %w", err)
	}

	preparedDevice := &ComputeDomainPreparedDevice{
		Device: drapbv1.Device{
			RequestNames: []string{"channel"},
			PoolName:     claim.Namespace,
			DeviceName:   deviceNameForChannel(0),
		},
		ContainerEdits: cdiEdits,
	}

	preparedDevices := ComputeDomainPreparedDevices{preparedDevice}
	if err := s.ensureClaimCDIArtifacts(claimUID, preparedDevices); err != nil {
		return nil, err
	}
	preparedClaims[claimUID] = preparedDevices

	if err := s.checkpointManager.CreateCheckpoint(DriverPluginCheckpointFile, checkpoint); err != nil {
		return nil, fmt.Errorf("unable to sync to checkpoint: %v", err)
	}

	return preparedDevices, nil
}

func (s *ComputeDomainState) Unprepare(claimUID string) error {
	s.Lock()
	defer s.Unlock()

	checkpoint := newComputeDomainCheckpoint()
	if err := s.checkpointManager.GetCheckpoint(DriverPluginCheckpointFile, checkpoint); err != nil {
		return fmt.Errorf("unable to sync from checkpoint: %v", err)
	}

	if checkpoint.V1 == nil {
		return nil
	}

	preparedClaims := checkpoint.V1.PreparedClaims
	domains := checkpoint.V1.Domains

	if preparedClaims[claimUID] == nil {
		return nil
	}

	for domainID, domainInfo := range domains {
		for i, claim := range domainInfo.Claims {
			if claim == claimUID {
				domainInfo.Claims = append(domainInfo.Claims[:i], domainInfo.Claims[i+1:]...)
				if len(domainInfo.Claims) == 0 && len(domainInfo.Pods) == 0 {
					delete(domains, domainID)
				}
				break
			}
		}
	}

	delete(preparedClaims, claimUID)

	if err := s.checkpointManager.CreateCheckpoint(DriverPluginCheckpointFile, checkpoint); err != nil {
		return fmt.Errorf("unable to sync to checkpoint: %v", err)
	}

	if err := s.cdi.DeleteClaimSpecFile(claimUID); err != nil {
		klog.Warningf("failed to delete CDI spec for claim %s: %v", claimUID, err)
	}

	return nil
}

func (s *ComputeDomainState) getDeviceClassName(claim *resourceapi.ResourceClaim) string {
	if claim.Spec.Devices.Requests != nil {
		for _, req := range claim.Spec.Devices.Requests {
			if req.Exactly != nil {
				return req.Exactly.DeviceClassName
			}
		}
	}
	return ""
}

func (s *ComputeDomainState) extractComputeDomainID(claim *resourceapi.ResourceClaim) string {
	// Extract domainID from device config parameters
	for _, config := range claim.Spec.Devices.Config {
		if config.Opaque != nil && config.Opaque.Driver == consts.ComputeDomainDriverName {
			// Parse the JSON parameters to extract domainID
			var params map[string]interface{}
			if err := json.Unmarshal(config.Opaque.Parameters.Raw, &params); err == nil {
				if domainID, ok := params["domainID"].(string); ok {
					return domainID
				}
			}
		}
	}
	return ""
}

func (s *ComputeDomainState) getOrCreateDomain(domains map[string]*DomainInfo, computeDomainID string, claim *resourceapi.ResourceClaim) string {
	// Check if domain already exists
	if domainInfo, exists := domains[computeDomainID]; exists {
		domainInfo.Claims = append(domainInfo.Claims, string(claim.UID))
		if !contains(domainInfo.Nodes, s.nodeName) {
			domainInfo.Nodes = append(domainInfo.Nodes, s.nodeName)
		}
		return computeDomainID
	}

	// Create new domain
	domainInfo := &DomainInfo{
		DomainID:          computeDomainID,
		ComputeDomainName: computeDomainID, // Use domainID as name
		ComputeDomainUID:  types.UID(computeDomainID),
		Nodes:             []string{s.nodeName},
		Pods:              []types.NamespacedName{},
		Claims:            []string{string(claim.UID)},
		CreatedAt:         time.Now(),
	}

	domains[computeDomainID] = domainInfo
	return computeDomainID
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (s *ComputeDomainState) ensureClaimCDIArtifacts(claimUID string, devices ComputeDomainPreparedDevices) error {
	if len(devices) == 0 {
		return fmt.Errorf("no devices prepared for claim %s", claimUID)
	}
	for _, device := range devices {
		ids := s.cdi.GetClaimDevices(claimUID, []string{device.DeviceName})
		if len(ids) == 0 {
			return fmt.Errorf("failed to build CDI device IDs for claim %s", claimUID)
		}
		device.CDIDeviceIDs = ids
	}

	if err := s.cdi.CreateClaimSpecFile(claimUID, devices); err != nil {
		return fmt.Errorf("unable to create CDI spec for claim %s: %w", claimUID, err)
	}

	return nil
}
