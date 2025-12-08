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

package consts

const DriverName = "gpu.example.com"

// ComputeDomain constants - matching NVIDIA's implementation
const (
	// ComputeDomainDriverName is the driver name for ComputeDomain resources
	// Same as NVIDIA's real implementation to ensure identical usage
	ComputeDomainDriverName = "compute-domain.nvidia.com"

	// ComputeDomainWorkloadDeviceClass is the DeviceClass for workload ResourceClaimTemplates
	ComputeDomainWorkloadDeviceClass = "compute-domain-default-channel.nvidia.com"

	// NVLinkCliqueLabel is the node label used to identify NVLink topology
	// Matches NVIDIA's real implementation exactly
	NVLinkCliqueLabel = "nvidia.com/gpu.clique"

	// ComputeDomainFinalizer is the finalizer added to ComputeDomain CRs
	ComputeDomainFinalizer = "computedomain.resource.nvidia.com/finalizer"
)
