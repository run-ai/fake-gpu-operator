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
	"fmt"

	resourceapi "k8s.io/api/resource/v1"
)

const (
	minChannelID = 0
	maxChannelID = 2048
)

type AllocatableComputeDomainDevices map[string]resourceapi.Device

// enumerateComputeDomainDevices creates fake domain devices for ComputeDomain
// Each node has one domain device representing domain membership capability
func enumerateComputeDomainDevices() (AllocatableComputeDomainDevices, error) {
	alldevices := make(AllocatableComputeDomainDevices)

	for _, channelID := range enumerateChannelIDs() {
		device := newChannelDevice(channelID)
		alldevices[device.Name] = device
	}

	return alldevices, nil
}

func enumerateChannelIDs() []int64 {
	count := maxChannelID - minChannelID + 1
	ids := make([]int64, 0, count)
	for id := minChannelID; id <= maxChannelID; id++ {
		ids = append(ids, int64(id))
	}
	return ids
}

func newChannelDevice(channelID int64) resourceapi.Device {
	return resourceapi.Device{
		Name: deviceNameForChannel(channelID),
		Attributes: map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
			"compute-domain.nvidia.com/type": {
				StringValue: stringPtr("channel"),
			},
			"compute-domain.nvidia.com/id": {
				IntValue: intPtr(channelID),
			},
		},
	}
}

func deviceNameForChannel(channelID int64) string {
	return fmt.Sprintf("channel-%d", channelID)
}

func stringPtr(s string) *string {
	return &s
}

func intPtr(i int64) *int64 {
	return &i
}
