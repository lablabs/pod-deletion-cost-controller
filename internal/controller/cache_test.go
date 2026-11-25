package controller_test

import (
	"context"
	"testing"

	"github.com/lablabs/pod-deletion-cost-controller/internal/controller"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestCacheOptionsStripManagedFields(t *testing.T) {
	opts := controller.CacheOptions()
	require.NotNil(t, opts.DefaultTransform)

	pod := &corev1.Pod{ObjectMeta: v1.ObjectMeta{
		Name:          "pod",
		ManagedFields: []v1.ManagedFieldsEntry{{Manager: "kubelet"}},
	}}
	out, err := opts.DefaultTransform(pod)
	require.NoError(t, err)
	require.Empty(t, out.(*corev1.Pod).ManagedFields)

	rs := controller.ReplicaSetMetadata()
	rs.ManagedFields = []v1.ManagedFieldsEntry{{Manager: "kube-controller-manager"}}
	out, err = opts.DefaultTransform(rs)
	require.NoError(t, err)
	require.Empty(t, out.(*v1.PartialObjectMetadata).ManagedFields)
}

func TestGetDeploymentFromMetadata(t *testing.T) {
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, appsv1.AddToScheme(s))

	dep := &appsv1.Deployment{ObjectMeta: v1.ObjectMeta{
		Name:      "web",
		Namespace: "default",
		UID:       "dep-uid",
		Annotations: map[string]string{
			controller.EnableAnnotation: "true",
			controller.TypeAnnotation:   "zone",
		},
	}}
	rs := &appsv1.ReplicaSet{ObjectMeta: v1.ObjectMeta{
		Name:      "web-abc",
		Namespace: "default",
		UID:       "rs-uid",
		OwnerReferences: []v1.OwnerReference{{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       dep.Name,
			UID:        dep.UID,
		}},
	}}
	pod := &corev1.Pod{ObjectMeta: v1.ObjectMeta{
		Name:      "web-abc-1",
		Namespace: "default",
		OwnerReferences: []v1.OwnerReference{{
			APIVersion: "apps/v1",
			Kind:       "ReplicaSet",
			Name:       rs.Name,
			UID:        rs.UID,
		}},
	}}
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(dep, rs).Build()

	got, err := controller.GetDeployment(context.Background(), c, pod)
	require.NoError(t, err)
	require.Equal(t, dep.Name, got.Name)
	require.Equal(t, dep.UID, got.UID)
	require.True(t, controller.IsEnabled(got))
	require.Equal(t, "zone", controller.GetType(got))
}

func TestDeploymentPredicateMetadata(t *testing.T) {
	pred := controller.DeploymentPredicate()

	enabled := controller.DeploymentMetadata()
	enabled.Annotations = map[string]string{controller.EnableAnnotation: "true"}
	require.True(t, pred.Create(event.CreateEvent{Object: enabled}))

	disabled := controller.DeploymentMetadata()
	require.False(t, pred.Create(event.CreateEvent{Object: disabled}))
}

func TestTransformPod(t *testing.T) {
	now := v1.Now()
	pod := &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:              "pod",
			Namespace:         "default",
			UID:               "pod-uid",
			CreationTimestamp: now,
			DeletionTimestamp: &now,
			Labels:            map[string]string{"app": "web"},
			Annotations:       map[string]string{controller.PodDeletionCostAnnotation: "10"},
			OwnerReferences:   []v1.OwnerReference{{Kind: "ReplicaSet", Name: "web-abc", UID: "rs-uid"}},
			ManagedFields:     []v1.ManagedFieldsEntry{{Manager: "kubelet"}},
		},
		Spec: corev1.PodSpec{
			NodeName:   "node-a",
			Containers: []corev1.Container{{Name: "web", Image: "nginx"}},
		},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			ContainerStatuses: []corev1.ContainerStatus{{Name: "web"}},
			PodIP:             "10.0.0.1",
		},
	}

	want := pod.DeepCopy()
	want.ManagedFields = nil
	want.Spec = corev1.PodSpec{NodeName: "node-a"}
	want.Status = corev1.PodStatus{
		Phase:      corev1.PodRunning,
		Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
	}

	out, err := controller.TransformPod(pod)
	require.NoError(t, err)
	require.Equal(t, want, out)

	out, err = controller.TransformPod(out)
	require.NoError(t, err)
	require.Equal(t, want, out)

	other := controller.NodeMetadata()
	out, err = controller.TransformPod(other)
	require.NoError(t, err)
	require.Same(t, other, out)
}

func TestCacheOptionsPodTransform(t *testing.T) {
	opts := controller.CacheOptions()
	var found bool
	for obj, byObject := range opts.ByObject {
		if _, ok := obj.(*corev1.Pod); ok {
			require.NotNil(t, byObject.Transform)
			found = true
		}
	}
	require.True(t, found)
}

func TestMergePatchFromTransformedPod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{Name: "pod", Namespace: "default"},
		Spec:       corev1.PodSpec{NodeName: "node-a", Containers: []corev1.Container{{Name: "web"}}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	out, err := controller.TransformPod(pod)
	require.NoError(t, err)
	cached := out.(*corev1.Pod)

	patch := client.MergeFrom(cached.DeepCopy())
	controller.ApplyPodDeletionCost(cached, 5)
	data, err := patch.Data(cached)
	require.NoError(t, err)
	require.JSONEq(t, `{"metadata":{"annotations":{"controller.kubernetes.io/pod-deletion-cost":"5"}}}`, string(data))
}
