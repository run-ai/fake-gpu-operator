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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	resourceapi "k8s.io/api/resource/v1"
)

func TestEnumerateComputeDomainDevices(t *testing.T) {
	devices, err := enumerateComputeDomainDevices()
	require.NoError(t, err)
	expectedChannels := maxChannelID
	assert.Len(t, devices, expectedChannels)

	device, exists := devices[deviceNameForChannel(0)]
	require.True(t, exists, "channel 0 device should exist")
	assert.Equal(t, deviceNameForChannel(0), device.Name)
	assert.NotNil(t, device.Attributes)

	// Check attributes
	typeAttr, exists := device.Attributes["compute-domain.nvidia.com/type"]
	assert.True(t, exists)
	assert.NotNil(t, typeAttr.StringValue)
	assert.Equal(t, "channel", *typeAttr.StringValue)

	idAttr, exists := device.Attributes["compute-domain.nvidia.com/id"]
	assert.True(t, exists)
	assert.NotNil(t, idAttr.IntValue)
	assert.Equal(t, int64(0), *idAttr.IntValue)
}

func TestAllocatableComputeDomainDevices(t *testing.T) {
	devices := make(AllocatableComputeDomainDevices)
	assert.NotNil(t, devices)
	assert.Len(t, devices, 0)

	device := resourceapi.Device{
		Name: "test-domain",
	}
	devices["test-domain"] = device

	assert.Len(t, devices, 1)
	assert.Equal(t, device, devices["test-domain"])
}
