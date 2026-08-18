package timestamprank

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/go-logr/logr"
	"github.com/lablabs/pod-deletion-cost-controller/internal/controller"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// TypeAnnotation is the algorithm type identifier for the timestamp-rank handler.
	TypeAnnotation = "timestamp-rank"

	// ManagedByAnnotation records which algorithm produced the current
	// pod-deletion-cost value. It lets this handler tell a rank it wrote apart
	// from a value left behind by another algorithm (e.g. "zone") after a
	// Deployment switches type, so the foreign value is replaced rather than
	// trusted.
	//
	// Note the reverse switch is not symmetric: "zone" returns early for any pod
	// that already carries a cost, so it will not reclaim pods ranked here. See
	// the README section on switching a Deployment's type.
	ManagedByAnnotation = "pod-deletion-cost.lablabs.io/managed-by"

	// firstRank is the cost assigned to the oldest pod of a ReplicaSet.
	//
	// Ranks are 1-based on purpose: a pod without the annotation is read by the
	// ReplicaSet controller as cost 0, so starting at 1 keeps every ranked pod
	// above the not-yet-ranked ones. A freshly created pod is therefore shed
	// before an already ranked, older pod during the window between its creation
	// and its first reconcile.
	firstRank = 1

	// maxRankedPods bounds the ranking to keep every cost inside the int32 range
	// the ReplicaSet controller parses the annotation into. Pods beyond this
	// bound all receive MaxInt32 (least likely to be deleted) instead of
	// overflowing into a value that would be silently read as 0. A ReplicaSet
	// this large is pathological; the guard only exists so the failure mode is
	// "loses relative ordering" rather than "inverts it".
	maxRankedPods = math.MaxInt32
)

// NewHandler creates a new timestamp-rank based Handler.
func NewHandler(client client.Client) *Handler {
	return &Handler{client: client}
}

// Handler assigns pod-deletion-cost by ranking the pods of a ReplicaSet against
// each other by age. The oldest pod gets the lowest cost and is deleted first
// during scale-down, which drains the oldest pods of a fleet through everyday
// scaling operations.
//
// Unlike the "timestamp" algorithm — which derives an absolute cost from a single
// pod's CreationTimestamp — this handler compares pods to each other, so every
// pod in a ReplicaSet gets a distinct cost even when several were created within
// the same second. CreationTimestamp is serialized as RFC3339 and therefore has
// no sub-second component, so no arithmetic on a single pod's timestamp can
// separate pods created in the same second; only a relative ranking can.
type Handler struct {
	client client.Client
}

// AcceptType returns the algorithm type identifiers this handler responds to.
func (h *Handler) AcceptType() []string {
	return []string{TypeAnnotation}
}

// Handle ranks all pods of the reconciled pod's ReplicaSet by age and stores each
// pod's rank as its pod-deletion-cost.
//
// The ranking is a pure function of the ReplicaSet's pod set, ordered by
// (CreationTimestamp, UID). Both inputs are immutable, so every concurrent
// reconcile — no matter which pod triggered it or in which order the API returns
// the pods — computes the same cost for the same pod. That determinism is what
// makes an expectations cache unnecessary here: repeated patches are idempotent
// rather than racing to claim distinct values from a shared pool the way the
// "zone" algorithm's descending allocation does.
//
// Pods whose cost already matches their computed rank are left alone. Each
// pod-deletion-cost write re-triggers ReplicaSet reconciliation, and KEP-2255
// explicitly warns against updating the annotation frequently, so a reconcile
// that changes nothing must issue no writes at all.
func (h *Handler) Handle(ctx context.Context, log logr.Logger, pod *corev1.Pod, _ *appv1.Deployment) error {
	if controller.IsDeleting(pod) {
		return nil
	}

	podList := &corev1.PodList{}
	if err := controller.ListPodsByOwnerRSIndex(ctx, h.client, pod, podList); err != nil {
		return fmt.Errorf("unable to list pods by rs: %w", err)
	}

	ranked := rankable(podList.Items)
	if len(ranked) == 0 {
		// The pod has no owning ReplicaSet, or the index has not caught up yet.
		// Either way there is nothing to rank against; a later event re-runs this.
		log.V(3).Info("no pods to rank")
		return nil
	}

	sortByAge(ranked)

	for i := range ranked {
		target := &ranked[i]
		cost := rankCost(i)

		if current, ok := controller.GetPodDeletionCost(target); ok && current == cost && managedByTimestampRank(target) {
			continue
		}

		patch := client.MergeFrom(target.DeepCopy())
		controller.ApplyPodDeletionCost(target, cost)
		setManagedBy(target)
		if err := h.client.Patch(ctx, target, patch); err != nil {
			return fmt.Errorf("unable to patch pod %s/%s: %w", target.Namespace, target.Name, err)
		}

		log.WithValues("pod", target.Name, controller.PodDeletionCostAnnotation, cost).Info("updated")
	}

	return nil
}

// rankable returns the pods eligible for ranking: everything that is not already
// terminating. A pod with a DeletionTimestamp is on its way out and must not
// consume a rank, otherwise it would shift the ranks of all younger pods and
// cause a pointless rewrite of the whole ReplicaSet once it disappears.
//
// Not-Ready pods are deliberately kept. The controller's predicate only decides
// which pods *enqueue* a reconcile; the listing sees every pod, and ranking all
// of them keeps ranks stable as pods become ready. Kubernetes also compares
// phase and readiness before pod-deletion-cost when picking a victim, so an
// unready pod is already preferred for deletion regardless of its rank.
func rankable(pods []corev1.Pod) []corev1.Pod {
	out := make([]corev1.Pod, 0, len(pods))
	for _, p := range pods {
		if controller.IsDeleting(&p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// sortByAge orders pods oldest-first. UID breaks ties so that pods created within
// the same second still get a stable, total order — without the tie-break the
// result would depend on the order the API server returned the pods in and
// concurrent reconciles could disagree on the ranks.
func sortByAge(pods []corev1.Pod) {
	sort.Slice(pods, func(i, j int) bool {
		ti, tj := pods[i].CreationTimestamp.Time, pods[j].CreationTimestamp.Time
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return pods[i].UID < pods[j].UID
	})
}

// rankCost maps a 0-based position in the age-sorted list to a deletion cost,
// clamped to the int32 range the ReplicaSet controller parses the annotation into.
func rankCost(index int) int {
	if index >= maxRankedPods-firstRank {
		return math.MaxInt32
	}
	return index + firstRank
}

// managedByTimestampRank reports whether the pod's current cost was set by this handler.
func managedByTimestampRank(pod *corev1.Pod) bool {
	if pod.Annotations == nil {
		return false
	}
	return pod.Annotations[ManagedByAnnotation] == TypeAnnotation
}

// setManagedBy stamps the pod as owned by the timestamp-rank algorithm.
func setManagedBy(pod *corev1.Pod) {
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[ManagedByAnnotation] = TypeAnnotation
}
