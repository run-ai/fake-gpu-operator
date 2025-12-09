package dra_plugin_gpu

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	drapbv1 "k8s.io/kubelet/pkg/apis/dra/v1beta1"
)

func TestNewCheckpoint(t *testing.T) {
	checkpoint := newCheckpoint()

	require.NotNil(t, checkpoint)
	require.NotNil(t, checkpoint.V1)
	require.NotNil(t, checkpoint.V1.PreparedClaims)
	assert.Empty(t, checkpoint.V1.PreparedClaims)
}

func TestCheckpoint_MarshalCheckpoint(t *testing.T) {
	tests := map[string]struct {
		checkpoint *Checkpoint
		wantErr    bool
	}{
		"empty checkpoint": {
			checkpoint: newCheckpoint(),
			wantErr:    false,
		},
		"checkpoint with prepared claims": {
			checkpoint: &Checkpoint{
				V1: &CheckpointV1{
					PreparedClaims: PreparedClaims{
						"claim-1": PreparedDevices{
							{
								Device: drapbv1.Device{
									DeviceName: "gpu-0",
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			data, err := test.checkpoint.MarshalCheckpoint()
			if test.wantErr {
				assert.Error(t, err)
				assert.Nil(t, data)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, data)
				// Verify it's valid JSON
				var decoded Checkpoint
				err := json.Unmarshal(data, &decoded)
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckpoint_UnmarshalCheckpoint(t *testing.T) {
	tests := map[string]struct {
		data    []byte
		wantErr bool
	}{
		"valid empty checkpoint": {
			data:    []byte(`{"v1":{"preparedClaims":{}}}`),
			wantErr: false,
		},
		"valid checkpoint with claims": {
			data:    []byte(`{"v1":{"preparedClaims":{"claim-1":[]}}}`),
			wantErr: false,
		},
		"invalid JSON": {
			data:    []byte(`{invalid json}`),
			wantErr: true,
		},
		"empty data": {
			data:    []byte(``),
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			checkpoint := newCheckpoint()
			err := checkpoint.UnmarshalCheckpoint(test.data)
			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckpoint_VerifyChecksum(t *testing.T) {
	checkpoint := newCheckpoint()
	err := checkpoint.VerifyChecksum()
	assert.NoError(t, err)

	// Test with non-empty checkpoint
	checkpoint.V1.PreparedClaims["test"] = PreparedDevices{}
	err = checkpoint.VerifyChecksum()
	assert.NoError(t, err)
}
