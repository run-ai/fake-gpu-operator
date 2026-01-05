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

package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s.io/apimachinery/pkg/types"
	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager/checksum"
)

func TestNewComputeDomainCheckpoint(t *testing.T) {
	checkpoint := newComputeDomainCheckpoint()
	require.NotNil(t, checkpoint)
	assert.Equal(t, checksum.Checksum(0), checkpoint.Checksum)
	assert.NotNil(t, checkpoint.V1)
	assert.NotNil(t, checkpoint.V1.PreparedClaims)
	assert.NotNil(t, checkpoint.V1.Domains)
}

func TestComputeDomainCheckpoint_MarshalCheckpoint(t *testing.T) {
	checkpoint := newComputeDomainCheckpoint()
	checkpoint.V1.PreparedClaims["test-claim"] = ComputeDomainPreparedDevices{
		{
			Device: drapbv1.Device{
				DeviceName: "domain",
			},
		},
	}

	data, err := checkpoint.MarshalCheckpoint()
	require.NoError(t, err)
	assert.NotNil(t, data)
	assert.NotEqual(t, checksum.Checksum(0), checkpoint.Checksum)
}

func TestComputeDomainCheckpoint_UnmarshalCheckpoint(t *testing.T) {
	checkpoint := newComputeDomainCheckpoint()
	checkpoint.V1.PreparedClaims["test-claim"] = ComputeDomainPreparedDevices{
		{
			Device: drapbv1.Device{
				DeviceName: "domain",
			},
		},
	}

	data, err := checkpoint.MarshalCheckpoint()
	require.NoError(t, err)

	newCheckpoint := newComputeDomainCheckpoint()
	err = newCheckpoint.UnmarshalCheckpoint(data)
	require.NoError(t, err)
	assert.Equal(t, checkpoint.Checksum, newCheckpoint.Checksum)
	assert.NotNil(t, newCheckpoint.V1)
	assert.NotNil(t, newCheckpoint.V1.PreparedClaims)
}

func TestComputeDomainCheckpoint_VerifyChecksum(t *testing.T) {
	checkpoint := newComputeDomainCheckpoint()
	checkpoint.V1.PreparedClaims["test-claim"] = ComputeDomainPreparedDevices{
		{
			Device: drapbv1.Device{
				DeviceName: "domain",
			},
		},
	}

	_, err := checkpoint.MarshalCheckpoint()
	require.NoError(t, err)

	err = checkpoint.VerifyChecksum()
	assert.NoError(t, err)
}

func TestComputeDomainCheckpoint_VerifyChecksum_Invalid(t *testing.T) {
	checkpoint := newComputeDomainCheckpoint()
	checkpoint.V1.PreparedClaims["test-claim"] = ComputeDomainPreparedDevices{
		{
			Device: drapbv1.Device{
				DeviceName: "domain",
			},
		},
	}

	_, err := checkpoint.MarshalCheckpoint()
	require.NoError(t, err)

	// Corrupt checksum
	checkpoint.Checksum = checksum.Checksum(12345)

	err = checkpoint.VerifyChecksum()
	assert.Error(t, err)
}

func TestComputeDomainCheckpointV1_Domains(t *testing.T) {
	checkpoint := newComputeDomainCheckpoint()
	
	domainInfo := &DomainInfo{
		DomainID:          "test-domain-id",
		ComputeDomainName: "test-domain",
		ComputeDomainUID:  types.UID("test-uid"),
		Nodes:             []string{"node1"},
		Pods:              []types.NamespacedName{{Name: "pod1", Namespace: "default"}},
		Claims:            []string{"claim-uid"},
		CreatedAt:         time.Now(),
	}

	checkpoint.V1.Domains["test-domain-id"] = domainInfo

	data, err := json.Marshal(checkpoint)
	require.NoError(t, err)

	var unmarshaled ComputeDomainCheckpoint
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.NotNil(t, unmarshaled.V1.Domains)
	assert.Contains(t, unmarshaled.V1.Domains, "test-domain-id")
	assert.Equal(t, domainInfo.DomainID, unmarshaled.V1.Domains["test-domain-id"].DomainID)
}

