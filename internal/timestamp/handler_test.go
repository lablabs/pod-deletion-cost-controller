package timestamp_test

import (
	"context"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/lablabs/pod-deletion-cost-controller/internal/controller"
	"github.com/lablabs/pod-deletion-cost-controller/internal/timestamp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appv1.AddToScheme(scheme))
	return scheme
}

func enabledDeployment() *appv1.Deployment {
	return &appv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: "default",
			Annotations: map[string]string{
				"pod-deletion-cost.lablabs.io/enabled": "true",
				"pod-deletion-cost.lablabs.io/type":    "timestamp",
			},
		},
	}
}

func runPod(name string, creation time.Time) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(creation),
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

// handle runs the handler and returns the cost read back from the fake client
// (proving the patch was persisted, not only mutated in memory).
func handle(t *testing.T, pod *corev1.Pod) int {
	t.Helper()
	scheme := newScheme(t)
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	h := timestamp.NewHandler(client)
	log := zap.New(zap.UseDevMode(true))

	require.NoError(t, h.Handle(context.Background(), log, pod, enabledDeployment()))

	stored := &corev1.Pod{}
	require.NoError(t, client.Get(context.Background(), crclient.ObjectKeyFromObject(pod), stored))

	raw, ok := stored.Annotations[controller.PodDeletionCostAnnotation]
	require.True(t, ok, "expected pod-deletion-cost annotation to be persisted")
	// The managed-by stamp must be persisted alongside the cost.
	assert.Equal(t, "timestamp", stored.Annotations[timestamp.ManagedByAnnotation])

	cost, err := strconv.Atoi(raw)
	require.NoError(t, err)
	return cost
}

func TestHandler_AcceptType(t *testing.T) {
	h := timestamp.NewHandler(nil)
	assert.Equal(t, []string{"timestamp"}, h.AcceptType())
}

func TestHandler_Handle_SetsCostFromCreationTimestamp(t *testing.T) {
	creation := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	pod := runPod("test-pod", creation)

	cost := handle(t, pod)

	assert.Equal(t, int(creation.Unix()/60), cost)
}

func TestHandler_Handle_SkipsAlreadyAnnotatedByTimestamp(t *testing.T) {
	scheme := newScheme(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pod",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now()),
			Annotations: map[string]string{
				controller.PodDeletionCostAnnotation: "12345",
				timestamp.ManagedByAnnotation:        "timestamp",
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	h := timestamp.NewHandler(client)
	log := zap.New(zap.UseDevMode(true))

	require.NoError(t, h.Handle(context.Background(), log, pod, enabledDeployment()))
	assert.Equal(t, "12345", pod.Annotations[controller.PodDeletionCostAnnotation])
}

// TestHandler_Handle_RecalculatesForeignCost covers the zone→timestamp migration:
// a pod carrying a zone-generated cost (near MaxInt32) and no managed-by stamp must
// be recalculated so it is not left protected while newer timestamp pods get deleted.
func TestHandler_Handle_RecalculatesForeignCost(t *testing.T) {
	scheme := newScheme(t)

	creation := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "migrated-pod",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(creation),
			Annotations: map[string]string{
				// Leftover zone cost, no managed-by stamp.
				controller.PodDeletionCostAnnotation: strconv.Itoa(math.MaxInt32),
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	h := timestamp.NewHandler(client)
	log := zap.New(zap.UseDevMode(true))

	require.NoError(t, h.Handle(context.Background(), log, pod, enabledDeployment()))

	stored := &corev1.Pod{}
	require.NoError(t, client.Get(context.Background(), crclient.ObjectKeyFromObject(pod), stored))

	cost, err := strconv.Atoi(stored.Annotations[controller.PodDeletionCostAnnotation])
	require.NoError(t, err)
	assert.Equal(t, int(creation.Unix()/60), cost, "foreign cost should be recalculated to timestamp value")
	assert.Equal(t, "timestamp", stored.Annotations[timestamp.ManagedByAnnotation], "handler should take ownership")
}

func TestHandler_Handle_SkipsDeleting(t *testing.T) {
	scheme := newScheme(t)

	deletionTime := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-pod",
			Namespace:         "default",
			CreationTimestamp: metav1.NewTime(time.Now()),
			DeletionTimestamp: &deletionTime,
			Finalizers:        []string{"test-finalizer"},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	h := timestamp.NewHandler(client)
	log := zap.New(zap.UseDevMode(true))

	require.NoError(t, h.Handle(context.Background(), log, pod, enabledDeployment()))
	assert.Empty(t, pod.Annotations[controller.PodDeletionCostAnnotation])
}

func TestHandler_Handle_OlderPodsGetLowerCost(t *testing.T) {
	base := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	oldPod := runPod("old-pod", base.Add(-48*time.Hour))
	newPod := runPod("new-pod", base.Add(-1*time.Hour))

	oldCost := handle(t, oldPod)
	newCost := handle(t, newPod)

	assert.Less(t, oldCost, newCost, "older pod should have lower deletion cost")
}

// TestHandler_Handle_OrderingIndependentOfReconcileTime guards against the
// reconciliation-time-dependent bug: the cost must reflect absolute creation
// order, not the pod's age at the moment it happens to be reconciled.
func TestHandler_Handle_OrderingIndependentOfReconcileTime(t *testing.T) {
	podA := runPod("pod-a", time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)) // older
	podB := runPod("pod-b", time.Date(2026, 7, 13, 11, 0, 0, 0, time.UTC)) // newer

	// Reconcile order and any wall-clock delay is irrelevant: cost is a pure
	// function of CreationTimestamp.
	costB := handle(t, podB)
	costA := handle(t, podA)

	assert.Less(t, costA, costB, "older pod A must get a lower cost regardless of reconcile order/timing")
}

func TestHandler_Handle_Int32Safety(t *testing.T) {
	huge := time.Unix(int64(math.MaxInt32)*60+120, 0).UTC()
	pod := runPod("future-pod", huge)

	cost := handle(t, pod)
	assert.Equal(t, math.MaxInt32, cost, "far-future creation must clamp to MaxInt32")
}

func TestHandler_Handle_SameMinuteTie(t *testing.T) {
	base := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	p1 := runPod("p1", base.Add(5*time.Second))
	p2 := runPod("p2", base.Add(50*time.Second))

	c1 := handle(t, p1)
	c2 := handle(t, p2)

	assert.Equal(t, c1, c2, "pods created in the same minute bucket should tie")
}
