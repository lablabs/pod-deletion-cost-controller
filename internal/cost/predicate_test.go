package cost_test

import (
	"testing"

	"github.com/lablabs/pod-deletion-cost-controller/internal/cost"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestAcceptDeploymentPredicateFunc(t *testing.T) {
	p := cost.AcceptDeploymentPredicateFunc()

	tests := []struct {
		name     string
		obj      client.Object
		expected bool
	}{
		{
			name:     "non-deployment object → false",
			obj:      &corev1.Pod{},
			expected: false,
		},
		{
			name: "nil annotations → false",
			obj: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
			},
			expected: false,
		},
		{
			name: "missing annotation → false",
			obj: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			expected: false,
		},
		{
			name: "annotation = false → false",
			obj: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						cost.EnableAnnotation: "false",
					},
				},
			},
			expected: false,
		},
		{
			name: "annotation = true → true",
			obj: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						cost.EnableAnnotation: "true",
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := event.TypedCreateEvent[client.Object]{Object: tt.obj}
			result := p.Create(ev)
			if result != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestPredicateAcceptedPodFunc(t *testing.T) {
	p := cost.AcceptPodPredicateFunc()

	tests := []struct {
		name   string
		obj    client.Object
		expect bool
	}{
		{
			name:   "non-pod object -> false",
			obj:    &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
			expect: false,
		},
		{
			name: "pod without NodeName -> false",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod1"},
				Spec:       corev1.PodSpec{NodeName: ""},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning},
			},
			expect: false,
		},
		{
			name: "pod not running -> false",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod2"},
				Spec:       corev1.PodSpec{NodeName: "node1"},
				Status:     corev1.PodStatus{Phase: corev1.PodPending},
			},
			expect: false,
		},
		{
			name: "pod running but not ready -> false",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod3"},
				Spec:       corev1.PodSpec{NodeName: "node1"},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{Type: corev1.PodReady, Status: corev1.ConditionFalse},
					},
				},
			},
			expect: false,
		},
		{
			name: "pod ready but has deletion cost annotation -> false",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "pod4",
					Annotations: map[string]string{cost.PodDeletionCostAnnotation: "100"},
				},
				Spec: corev1.PodSpec{NodeName: "node1"},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{Type: corev1.PodReady, Status: corev1.ConditionTrue},
					},
				},
			},
			expect: false,
		},
		{
			name: "valid pod -> true",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "pod5",
					Annotations: map[string]string{},
				},
				Spec: corev1.PodSpec{NodeName: "node1"},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{Type: corev1.PodReady, Status: corev1.ConditionTrue},
					},
				},
			},
			expect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := event.TypedCreateEvent[client.Object]{Object: tt.obj}
			result := p.Create(ev) // predicateFuncs applies to all events equally
			if result != tt.expect {
				t.Fatalf("expected %v, got %v", tt.expect, result)
			}
		})
	}
}
