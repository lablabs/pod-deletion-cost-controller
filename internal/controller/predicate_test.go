package controller_test

import (
	"testing"
	"time"

	"github.com/lablabs/pod-deletion-cost-controller/internal/controller"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestAcceptDeployment(t *testing.T) {
	pred := controller.DeploymentPredicate()

	tests := []struct {
		name string
		dep  *appsv1.Deployment
		want bool
	}{
		{
			name: "nil annotations → false",
			dep:  &appsv1.Deployment{},
			want: false,
		},
		{
			name: "annotation disabled → false",
			dep: &appsv1.Deployment{
				ObjectMeta: controllerruntime.ObjectMeta{
					Annotations: map[string]string{
						controller.EnableAnnotation: "false",
					},
				},
			},
			want: false,
		},
		{
			name: "annotation enabled → true",
			dep: &appsv1.Deployment{
				ObjectMeta: controllerruntime.ObjectMeta{
					Annotations: map[string]string{
						controller.EnableAnnotation: "true",
					},
				},
			},
			want: true,
		},
		{
			name: "wrong object type → false",
			dep:  nil, // passed through obj.(*Deployment) path
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var obj interface{}
			if tt.dep != nil {
				obj = tt.dep
			} else {
				obj = &corev1.Pod{}
			}

			got := pred.Create(event.CreateEvent{Object: obj.(client.Object)})
			require.Equal(t, tt.want, got)
		})
	}
}

func TestAcceptPod(t *testing.T) {
	pred := controller.PodPredicate()

	now := v1.NewTime(time.Now())

	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{
			name: "pod is deleting → true",
			pod: &corev1.Pod{
				ObjectMeta: controllerruntime.ObjectMeta{
					DeletionTimestamp: &now,
				},
			},
			want: true,
		},
		{
			name: "pod not running → false",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodPending,
				},
			},
			want: false,
		},
		{
			name: "pod running but not ready → false",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{Type: corev1.PodReady, Status: corev1.ConditionFalse},
					},
				},
			},
			want: false,
		},
		{
			name: "pod running and ready → true",
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
					Conditions: []corev1.PodCondition{
						{Type: corev1.PodReady, Status: corev1.ConditionTrue},
					},
				},
			},
			want: true,
		},
		{
			name: "wrong object type (panic safe) → false",
			pod:  nil, // simulate non-Pod
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			var obj interface{}
			if tt.pod != nil {
				obj = tt.pod
			} else {
				obj = &appsv1.Deployment{}
			}

			got := pred.Create(event.CreateEvent{Object: obj.(client.Object)})
			require.Equal(t, tt.want, got)
		})
	}
}
