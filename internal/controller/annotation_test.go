package controller_test

import (
	testing "testing"

	"github.com/lablabs/pod-deletion-cost-controller/internal/controller"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{
			name:        "no annotations",
			annotations: nil,
			want:        false,
		},
		{
			name: "annotation present but not true",
			annotations: map[string]string{
				controller.EnableAnnotation: "false",
			},
			want: false,
		},
		{
			name: "annotation present with true",
			annotations: map[string]string{
				controller.EnableAnnotation: "true",
			},
			want: true,
		},
		{
			name: "annotation present but wrong value",
			annotations: map[string]string{
				controller.EnableAnnotation: "yes",
			},
			want: false,
		},
		{
			name: "other annotations but not target",
			annotations: map[string]string{
				"something": "else",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dep := &appsv1.Deployment{}
			dep.Annotations = tt.annotations

			got := controller.IsEnabled(dep)
			if got != tt.want {
				t.Errorf("IsEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasPodDeletionCost(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        bool
	}{
		{
			name:        "no annotations",
			annotations: nil,
			want:        false,
		},
		{
			name: "annotation present",
			annotations: map[string]string{
				controller.PodDeletionCostAnnotation: "100",
			},
			want: true,
		},
		{
			name: "annotation missing",
			annotations: map[string]string{
				"foo": "bar",
			},
			want: false,
		},
		{
			name: "annotation present with empty value",
			annotations: map[string]string{
				controller.PodDeletionCostAnnotation: "",
			},
			want: true, // presence is enough
		},
		{
			name: "multiple annotations including ours",
			annotations: map[string]string{
				"foo":                                "bar",
				controller.PodDeletionCostAnnotation: "50",
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{}
			pod.Annotations = tt.annotations

			got := controller.HasPodDeletionCost(pod)
			if got != tt.want {
				t.Errorf("HasPodDeletionCost() = %v, want %v", got, tt.want)
			}
		})
	}
}
