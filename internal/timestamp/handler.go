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

	// costEpochUnix is 2020-01-01T00:00:00Z expressed as a Unix timestamp.
	// The deletion cost is the number of seconds elapsed since this epoch, so
	// every pod created at a distinct second gets a distinct, monotonic cost
	// (older pod -> lower cost -> deleted first) — giving per-pod ordering
	// rather than a coarser bucket.
	//
	// pod-deletion-cost is stored as a free-form string annotation (the API
	// server does not range-check it). The int32 bound comes from the *reader*:
	// the ReplicaSet controller parses the value with strconv.ParseInt(_, 10, 32)
	// and, on overflow/parse error, silently treats the cost as 0 — which would
	// drop the pod's ordering. Offsetting from 2020 instead of the 1970 Unix
	// epoch keeps seconds-since-epoch inside int32 until ~2088 (vs the year-2038
	// boundary a raw Unix-seconds cost would hit), and deletionCost clamps
	// defensively so an out-of-range value degrades to the extreme rank rather
	// than collapsing to 0.
	costEpochUnix = 1577836800
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
// The cost is the number of seconds between costEpochUnix (2020-01-01) and the
// pod's creation time, clamped defensively to the int32 range the ReplicaSet
// controller parses the annotation into (values outside it are read as 0).
// The mapping is monotonic at one-second resolution: an earlier creation time
// always yields a lower-or-equal cost, so older pods are never ranked above
// newer ones. Resolution is limited to whole seconds because CreationTimestamp
// itself is only second-precision, so pods created within the same second get
// the same cost and are tie-broken by Kubernetes' other scale-down heuristics
// (this is far finer than the previous minute-level bucketing).
func deletionCost(creationUnix int64) int {
	cost := creationUnix - costEpochUnix

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
