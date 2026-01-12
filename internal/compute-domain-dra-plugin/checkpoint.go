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

	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager/checksum"
)

type ComputeDomainCheckpoint struct {
	Checksum checksum.Checksum          `json:"checksum"`
	V1       *ComputeDomainCheckpointV1 `json:"v1,omitempty"`
}

type ComputeDomainCheckpointV1 struct {
	PreparedClaims ComputeDomainPreparedClaims `json:"preparedClaims,omitempty"`
	Domains        map[string]*DomainInfo      `json:"domains,omitempty"`
}

func newComputeDomainCheckpoint() *ComputeDomainCheckpoint {
	pc := &ComputeDomainCheckpoint{
		Checksum: 0,
		V1: &ComputeDomainCheckpointV1{
			PreparedClaims: make(ComputeDomainPreparedClaims),
			Domains:        make(map[string]*DomainInfo),
		},
	}
	return pc
}

func (cp *ComputeDomainCheckpoint) MarshalCheckpoint() ([]byte, error) {
	cp.Checksum = 0
	out, err := json.Marshal(*cp)
	if err != nil {
		return nil, err
	}
	cp.Checksum = checksum.New(out)
	return json.Marshal(*cp)
}

func (cp *ComputeDomainCheckpoint) UnmarshalCheckpoint(data []byte) error {
	return json.Unmarshal(data, cp)
}

func (cp *ComputeDomainCheckpoint) VerifyChecksum() error {
	ck := cp.Checksum
	cp.Checksum = 0
	defer func() {
		cp.Checksum = ck
	}()
	out, err := json.Marshal(*cp)
	if err != nil {
		return err
	}
	return ck.Verify(out)
}
