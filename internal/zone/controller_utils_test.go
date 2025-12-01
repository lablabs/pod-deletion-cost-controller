package zone_test

import (
	"math"
	"testing"

	"github.com/lablabs/pod-deletion-cost-controller/internal/cost"
	"github.com/lablabs/pod-deletion-cost-controller/internal/zone"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodsWithCostAnnotation_LastAppliedCost(t *testing.T) {
	tests := []struct {
		name     string
		list     zone.DeletionCostList
		expected int
	}{
		{
			name:     "empty slice → MaxInt32",
			list:     zone.NewDeletionCostList([]corev1.Pod{}),
			expected: math.MaxInt32,
		},
		{
			name: "nil annotations → MaxInt32",
			list: zone.NewDeletionCostList([]corev1.Pod{
				corev1.Pod{
					ObjectMeta: v1.ObjectMeta{
						Annotations: nil,
					},
				},
			}),
			expected: math.MaxInt32,
		},
		{
			name: "annotation missing → MaxInt32",
			list: zone.NewDeletionCostList([]corev1.Pod{
				{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{},
					},
				},
			}),
			expected: math.MaxInt32,
		},
		{
			name: "annotation invalid (not int) → MaxInt32",
			list: zone.NewDeletionCostList([]corev1.Pod{
				{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{
							cost.PodDeletionCostAnnotation: "abc",
						},
					},
				},
			}),
			expected: math.MaxInt32,
		},
		{
			name: "valid annotation 42 cost → MaxInt32",
			list: zone.NewDeletionCostList([]corev1.Pod{
				{
					ObjectMeta: v1.ObjectMeta{
						Annotations: map[string]string{
							cost.PodDeletionCostAnnotation: "42",
						},
					},
				},
			}),
			expected: math.MaxInt32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := tt.list.FindNext()
			assert.NoError(t, err)
			if c != tt.expected {
				t.Fatalf("expected %d, got %d", tt.expected, c)
			}
		})
	}
}
