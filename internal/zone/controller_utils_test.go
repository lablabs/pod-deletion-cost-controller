package zone_test

import (
	"math"
	"testing"

	"github.com/lablabs/pod-deletion-cost-controller/internal/zone"
)

func TestDeletionCostPool_FindNextFree(t *testing.T) {
	tests := []struct {
		name        string
		initial     []int
		want        int
		expectError bool
	}{
		{
			name:        "empty pool -> returns MaxInt32",
			initial:     []int{},
			want:        math.MaxInt32,
			expectError: false,
		},
		{
			name:        "one value missing MaxInt32 -> returns MaxInt32",
			initial:     []int{100},
			want:        math.MaxInt32,
			expectError: false,
		},
		{
			name:        "MaxInt32 is taken -> returns next free below",
			initial:     []int{math.MaxInt32, math.MaxInt32 - 1},
			want:        math.MaxInt32 - 2,
			expectError: false,
		},
		{
			name:        "scattered pool -> returns highest missing number",
			initial:     []int{10, 20, math.MaxInt32 - 1},
			want:        math.MaxInt32,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := zone.NewDeletionCostPool()
			pool.AddValues(tt.initial)

			got, err := pool.FindNextFree()
			if (err != nil) != tt.expectError {
				t.Fatalf("expected error=%v, got %v", tt.expectError, err)
			}
			if err != nil {
				return
			}

			if got != tt.want {
				t.Fatalf("expected cost=%d, got %d", tt.want, got)
			}

			// Ensure returned slot is marked as used
			if _, exists := pool[got]; !exists {
				t.Fatalf("returned slot %d was not stored in pool", got)
			}
		})
	}
}
