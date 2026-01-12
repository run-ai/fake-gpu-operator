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

	resourceapi "k8s.io/api/resource/v1"
	"k8s.io/utils/ptr"
)

const (
	maxChannelID = 2048
)

type AllocatableComputeDomainDevices map[string]resourceapi.Device

func enumerateComputeDomainDevices() (AllocatableComputeDomainDevices, error) {
	alldevices := make(AllocatableComputeDomainDevices)

	for channelID := range maxChannelID {
		device := newChannelDevice(channelID)
		alldevices[device.Name] = device
	}

	return alldevices, nil
}

func newChannelDevice(channelID int) resourceapi.Device {
	return resourceapi.Device{
		Name: deviceNameForChannel(channelID),
		Attributes: map[resourceapi.QualifiedName]resourceapi.DeviceAttribute{
			"compute-domain.nvidia.com/type": {
				StringValue: ptr.To("channel"),
			},
			"compute-domain.nvidia.com/id": {
				IntValue: ptr.To(int64(channelID)),
			},
		},
	}
}

func deviceNameForChannel(channelID int) string {
	return fmt.Sprintf("channel-%d", channelID)
}
