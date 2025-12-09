package dra_plugin_gpu

import (
	"encoding/json"
)

// Checkpoint represents the checkpoint data structure for device state
type Checkpoint struct {
	V1 *CheckpointV1 `json:"v1"`
}

// CheckpointV1 represents version 1 of the checkpoint format
type CheckpointV1 struct {
	PreparedClaims PreparedClaims `json:"preparedClaims"`
}

// MarshalCheckpoint marshals the checkpoint to JSON
func (c *Checkpoint) MarshalCheckpoint() ([]byte, error) {
	return json.Marshal(c)
}

// UnmarshalCheckpoint unmarshals the checkpoint from JSON
func (c *Checkpoint) UnmarshalCheckpoint(data []byte) error {
	return json.Unmarshal(data, c)
}

// VerifyChecksum verifies the checksum of the checkpoint
func (c *Checkpoint) VerifyChecksum() error {
	// For now, we don't implement checksum verification
	// This can be enhanced later if needed
	return nil
}

// newCheckpoint creates a new empty checkpoint
func newCheckpoint() *Checkpoint {
	return &Checkpoint{
		V1: &CheckpointV1{
			PreparedClaims: make(PreparedClaims),
		},
	}
}
