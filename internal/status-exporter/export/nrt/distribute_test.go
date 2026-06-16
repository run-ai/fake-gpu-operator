package nrt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDistributeGPUs(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		zones    int
		explicit []int
		want     []int
		wantErr  bool
	}{
		{name: "even 8/2", count: 8, zones: 2, want: []int{4, 4}},
		{name: "even 8/4", count: 8, zones: 4, want: []int{2, 2, 2, 2}},
		{name: "remainder 7/2", count: 7, zones: 2, want: []int{4, 3}},
		{name: "single zone", count: 8, zones: 1, want: []int{8}},
		{name: "explicit uneven", count: 8, zones: 2, explicit: []int{6, 2}, want: []int{6, 2}},
		{name: "explicit sum mismatch", count: 8, zones: 2, explicit: []int{5, 2}, wantErr: true},
		{name: "explicit length mismatch", count: 8, zones: 3, explicit: []int{4, 4}, wantErr: true},
		{name: "zero zones", count: 8, zones: 0, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := distributeGPUs(tt.count, tt.zones, tt.explicit)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
