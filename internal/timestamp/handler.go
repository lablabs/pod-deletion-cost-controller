package timestamp

import (
	"context"
	"math"

	"github.com/go-logr/logr"
	"github.com/lablabs/pod-deletion-cost-controller/internal/controller"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// TypeAnnotation is the algorithm type identifier for the timestamp handler.
	TypeAnnotation = "timestamp"

	// ManagedByAnnotation records which algorithm produced the current
	// pod-deletion-cost value. It lets the timestamp handler distinguish a value
	// it owns (safe to skip) from a stale value written by another algorithm
	// (e.g. "zone"), which it must recalculate when a Deployment switches type.
	ManagedByAnnotation = "pod-deletion-cost.lablabs.io/managed-by"

	// costBucketSeconds is the time granularity (in seconds) used to derive the
	// deletion cost from the pod creation timestamp. Bucketing by the minute keeps
	// the resulting int32 value far below the API server limit (overflow ~year 6053),
	// avoiding the Year-2038 problem a raw unix timestamp would hit.
	costBucketSeconds = 60
)

// NewHandler creates a new timestamp-based Handler.
func NewHandler(client client.Client) *Handler {
	return &Handler{client: client}
}

// Handler assigns pod-deletion-cost based on pod creation time.
// Older pods get lower cost values and are deleted first during scale-down,
// enabling natural fleet refresh through everyday scaling operations.
type Handler struct {
	client client.Client
}

// AcceptType returns the algorithm type identifiers this handler responds to.
func (h *Handler) AcceptType() []string {
	return []string{TypeAnnotation}
}

// Handle sets the pod-deletion-cost annotation from the pod's creation timestamp.
// The cost is a pure function of CreationTimestamp (older pod → smaller timestamp →
// lower cost → deleted first), so the resulting ordering is independent of when the
// controller reconciles the pod. This matters because the annotation is written once
// and never recalculated: deriving the cost from the pod age at reconcile time would
// make a delayed or late-discovered pod receive a stale value and could invert the
// intended oldest-first order after a controller restart or reconciliation delay.
//
// A pod is skipped only when the timestamp handler already owns its cost. If the pod
// carries a cost written by a different algorithm (e.g. after a Deployment switches
// from "zone" to "timestamp"), the value is recalculated so the pod is not left with
// a stale, incompatible cost that would invert the deletion order.
func (h *Handler) Handle(ctx context.Context, log logr.Logger, pod *corev1.Pod, _ *appv1.Deployment) error {
	if controller.IsDeleting(pod) {
		return nil
	}

	if controller.HasPodDeletionCost(pod) && managedByTimestamp(pod) {
		return nil
	}

	cost := deletionCost(pod.CreationTimestamp.Unix())

	patch := client.MergeFrom(pod.DeepCopy())
	controller.ApplyPodDeletionCost(pod, cost)
	setManagedBy(pod)
	if err := h.client.Patch(ctx, pod, patch); err != nil {
		return err
	}

	log.WithValues(controller.PodDeletionCostAnnotation, cost).Info("updated")
	return nil
}

// deletionCost maps a pod creation unix timestamp to a pod-deletion-cost value.
// The timestamp is bucketed by costBucketSeconds so the value stays within the
// int32 range the Kubernetes API server enforces, then clamped defensively.
// The mapping is monotonic: an earlier creation time always yields a lower or
// equal cost, so older pods are never ranked above newer ones.
func deletionCost(creationUnix int64) int {
	cost := creationUnix / costBucketSeconds

	if cost > math.MaxInt32 {
		return math.MaxInt32
	}
	if cost < math.MinInt32 {
		return math.MinInt32
	}
	return int(cost)
}

// managedByTimestamp reports whether the pod's current cost was set by this handler.
func managedByTimestamp(pod *corev1.Pod) bool {
	if pod.Annotations == nil {
		return false
	}
	return pod.Annotations[ManagedByAnnotation] == TypeAnnotation
}

// setManagedBy stamps the pod as owned by the timestamp algorithm.
func setManagedBy(pod *corev1.Pod) {
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[ManagedByAnnotation] = TypeAnnotation
}
