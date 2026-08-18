package timestamprank_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/lablabs/pod-deletion-cost-controller/internal/controller"
	"github.com/lablabs/pod-deletion-cost-controller/internal/timestamprank"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	namespace = "default"
	rsUID     = types.UID("rs-uid-1")
	otherRS   = types.UID("rs-uid-2")
)

var base = time.Date(2026, 8, 11, 15, 52, 32, 0, time.UTC)

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
			Namespace: namespace,
			Annotations: map[string]string{
				"pod-deletion-cost.lablabs.io/enabled": "true",
				"pod-deletion-cost.lablabs.io/type":    "timestamp-rank",
			},
		},
	}
}

type podOpt func(*corev1.Pod)

func withCost(cost int) podOpt {
	return func(p *corev1.Pod) {
		controller.ApplyPodDeletionCost(p, cost)
	}
}

func withManagedBy(value string) podOpt {
	return func(p *corev1.Pod) {
		if p.Annotations == nil {
			p.Annotations = map[string]string{}
		}
		p.Annotations[timestamprank.ManagedByAnnotation] = value
	}
}

func withOwnerRS(uid types.UID) podOpt {
	return func(p *corev1.Pod) {
		p.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: "apps/v1",
			Kind:       "ReplicaSet",
			Name:       "test-rs",
			UID:        uid,
		}}
	}
}

func deleting() podOpt {
	return func(p *corev1.Pod) {
		now := metav1.NewTime(base)
		p.DeletionTimestamp = &now
		p.Finalizers = []string{"test-finalizer"}
	}
}

func notReady() podOpt {
	return func(p *corev1.Pod) {
		p.Status = corev1.PodStatus{Phase: corev1.PodPending}
	}
}

// pod builds a Running+Ready pod owned by rsUID, created at `creation`.
func pod(name string, creation time.Time, opts ...podOpt) *corev1.Pod {
	p := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			UID:               types.UID("uid-" + name),
			CreationTimestamp: metav1.NewTime(creation),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}
	withOwnerRS(rsUID)(p)
	for _, o := range opts {
		o(p)
	}
	return p
}

// newClient wires a fake client with the pod-to-ReplicaSet field index the
// handler lists through, and counts pod patches so "no-op means no writes" can
// be asserted.
func newClient(t *testing.T, pods ...*corev1.Pod) (crclient.Client, *int) {
	t.Helper()
	return buildClient(t, nil, nil, pods...)
}

// newClientWithListOrder additionally rewrites the order in which List returns
// the ReplicaSet's pods. The fake client sorts by name, so without this hook a
// "shuffled input" test would still see one canonical order and could not
// detect a non-deterministic sort.
func newClientWithListOrder(t *testing.T, reorder func([]corev1.Pod), pods ...*corev1.Pod) (crclient.Client, *int) {
	t.Helper()
	return buildClient(t, reorder, nil, pods...)
}

// newClientWithPatchError makes every pod patch fail with the given error.
func newClientWithPatchError(t *testing.T, patchErr error, pods ...*corev1.Pod) (crclient.Client, *int) {
	t.Helper()
	return buildClient(t, nil, patchErr, pods...)
}

func buildClient(t *testing.T, reorder func([]corev1.Pod), patchErr error, pods ...*corev1.Pod) (crclient.Client, *int) {
	t.Helper()

	objs := make([]crclient.Object, 0, len(pods))
	for _, p := range pods {
		objs = append(objs, p.DeepCopy())
	}

	patches := 0
	c := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithObjects(objs...).
		WithIndex(&corev1.Pod{}, controller.PodToRSIndex, func(obj crclient.Object) []string {
			p := obj.(*corev1.Pod)
			for _, owner := range p.OwnerReferences {
				if owner.Kind == "ReplicaSet" {
					return []string{string(owner.UID)}
				}
			}
			return nil
		}).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c crclient.WithWatch, list crclient.ObjectList, opts ...crclient.ListOption) error {
				if err := c.List(ctx, list, opts...); err != nil {
					return err
				}
				if podList, ok := list.(*corev1.PodList); ok && reorder != nil {
					reorder(podList.Items)
				}
				return nil
			},
			Patch: func(ctx context.Context, c crclient.WithWatch, obj crclient.Object, patch crclient.Patch, opts ...crclient.PatchOption) error {
				if _, ok := obj.(*corev1.Pod); ok {
					patches++
				}
				if patchErr != nil {
					return patchErr
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	return c, &patches
}

// costs reads back the persisted pod-deletion-cost of every named pod, proving
// the handler patched the API objects and not just its in-memory copies.
func costs(t *testing.T, c crclient.Client, names ...string) map[string]int {
	t.Helper()

	out := make(map[string]int, len(names))
	for _, name := range names {
		stored := &corev1.Pod{}
		require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, stored))
		raw, ok := stored.Annotations[controller.PodDeletionCostAnnotation]
		if !ok {
			continue
		}
		cost, err := strconv.Atoi(raw)
		require.NoError(t, err)
		out[name] = cost
	}
	return out
}

func handle(t *testing.T, c crclient.Client, trigger *corev1.Pod) {
	t.Helper()
	h := timestamprank.NewHandler(c)
	log := zap.New(zap.UseDevMode(true))
	require.NoError(t, h.Handle(context.Background(), log, trigger, enabledDeployment()))
}

func TestHandler_AcceptType(t *testing.T) {
	h := timestamprank.NewHandler(nil)
	assert.Equal(t, []string{"timestamp-rank"}, h.AcceptType())
}

func TestHandler_Handle_OlderPodGetsLowerCost(t *testing.T) {
	old := pod("old", base.Add(-48*time.Hour))
	recent := pod("recent", base.Add(-1*time.Hour))

	c, _ := newClient(t, old, recent)
	handle(t, c, recent)

	got := costs(t, c, "old", "recent")
	assert.Equal(t, 1, got["old"])
	assert.Equal(t, 2, got["recent"])
	assert.Less(t, got["old"], got["recent"], "older pod must be deleted first")
}

// TestHandler_Handle_SameSecondBurstGetsDistinctCosts is the reason this
// algorithm exists. Scaling a Deployment 1 -> 11 in a single kubectl scale
// produces pods that share a creationTimestamp second, which the absolute
// "timestamp" algorithm cannot separate (11 pods, 4 unique costs). Ranking must
// yield 11 pods with 11 strictly increasing costs.
func TestHandler_Handle_SameSecondBurstGetsDistinctCosts(t *testing.T) {
	// Mirrors the observed burst: one pod at :32, then two at :34, four at :35,
	// four at :36.
	offsets := []int{0, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4}
	pods := make([]*corev1.Pod, 0, len(offsets))
	names := make([]string, 0, len(offsets))
	for i, off := range offsets {
		name := fmt.Sprintf("burst-%02d", i)
		names = append(names, name)
		pods = append(pods, pod(name, base.Add(time.Duration(off)*time.Second)))
	}

	c, _ := newClient(t, pods...)
	handle(t, c, pods[0])

	got := costs(t, c, names...)
	require.Len(t, got, len(offsets), "every pod must be ranked")

	unique := make(map[int]struct{}, len(got))
	for _, cost := range got {
		unique[cost] = struct{}{}
	}
	assert.Len(t, unique, len(offsets), "pods created in the same second must get distinct costs")

	// Pods created in the same second are ordered among themselves, and every
	// older second still ranks strictly below every newer one.
	for i := 1; i < len(names); i++ {
		prev, cur := got[names[i-1]], got[names[i]]
		if offsets[i-1] == offsets[i] {
			assert.NotEqual(t, prev, cur, "same-second pods %s/%s must differ", names[i-1], names[i])
			continue
		}
		assert.Less(t, prev, cur, "%s (older second) must rank below %s", names[i-1], names[i])
	}
}

// TestHandler_Handle_DeterministicRegardlessOfTrigger proves an expectations
// cache is unnecessary: whichever pod triggers the reconcile, and in whatever
// order the API returns the pods, the computed costs are identical.
func TestHandler_Handle_DeterministicRegardlessOfTrigger(t *testing.T) {
	sameSecond := base
	pods := []*corev1.Pod{
		pod("pod-a", sameSecond),
		pod("pod-b", sameSecond),
		pod("pod-d", sameSecond),
		pod("pod-e", base.Add(time.Second)),
	}
	names := []string{"pod-a", "pod-b", "pod-d", "pod-e"}

	// Every rotation of the listed pods, combined with a different triggering
	// pod each round.
	var want map[string]int
	for round := range pods {
		shift := round
		c, _ := newClientWithListOrder(t, func(items []corev1.Pod) {
			rotate(items, shift)
		}, pods...)
		handle(t, c, pods[round])

		got := costs(t, c, names...)
		if want == nil {
			want = got
			continue
		}
		assert.Equal(t, want, got, "ranking must not depend on list or reconcile order")
	}

	// Sanity: the UID tie-break keeps same-second pods in name order here,
	// because each pod's UID is derived from its name.
	assert.Equal(t, map[string]int{"pod-a": 1, "pod-b": 2, "pod-d": 3, "pod-e": 4}, want)
}

// rotate shifts a slice left by n, giving a cheap deterministic reordering.
func rotate(items []corev1.Pod, n int) {
	if len(items) == 0 {
		return
	}
	n %= len(items)
	rotated := append(append([]corev1.Pod{}, items[n:]...), items[:n]...)
	copy(items, rotated)
}

// TestHandler_Handle_NoWritesWhenAlreadyCorrect guards the KEP-2255 warning
// about frequent pod-deletion-cost updates: a converged reconcile must not
// re-patch anything.
func TestHandler_Handle_NoWritesWhenAlreadyCorrect(t *testing.T) {
	old := pod("old", base, withCost(1), withManagedBy("timestamp-rank"))
	recent := pod("recent", base.Add(time.Minute), withCost(2), withManagedBy("timestamp-rank"))

	c, patches := newClient(t, old, recent)
	handle(t, c, recent)

	assert.Zero(t, *patches, "a no-op reconcile must issue zero writes")
	assert.Equal(t, map[string]int{"old": 1, "recent": 2}, costs(t, c, "old", "recent"))
}

// TestHandler_Handle_PatchesOnlyChangedPods complements the no-op case: a pod
// that is already correct is left alone even when a sibling needs a rewrite.
func TestHandler_Handle_PatchesOnlyChangedPods(t *testing.T) {
	old := pod("old", base, withCost(1), withManagedBy("timestamp-rank"))
	recent := pod("recent", base.Add(time.Minute))

	c, patches := newClient(t, old, recent)
	handle(t, c, recent)

	assert.Equal(t, 1, *patches, "only the pod whose cost changed may be patched")
	assert.Equal(t, map[string]int{"old": 1, "recent": 2}, costs(t, c, "old", "recent"))
}

// TestHandler_Handle_RewritesForeignCost covers a Deployment switching from
// "zone" to "timestamp-rank": a leftover cost with no managed-by stamp must be
// replaced, otherwise the pod keeps a value from an incompatible scale.
func TestHandler_Handle_RewritesForeignCost(t *testing.T) {
	old := pod("old", base, withCost(2147483647))
	recent := pod("recent", base.Add(time.Minute), withCost(2147483646))

	c, _ := newClient(t, old, recent)
	handle(t, c, recent)

	assert.Equal(t, map[string]int{"old": 1, "recent": 2}, costs(t, c, "old", "recent"))

	stored := &corev1.Pod{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: "old"}, stored))
	assert.Equal(t, "timestamp-rank", stored.Annotations[timestamprank.ManagedByAnnotation])
}

// TestHandler_Handle_ExcludesTerminatingPods keeps a terminating pod from
// consuming a rank, which would otherwise shift every younger pod and force a
// full rewrite once the pod disappears.
func TestHandler_Handle_ExcludesTerminatingPods(t *testing.T) {
	dying := pod("dying", base.Add(-time.Hour), deleting())
	old := pod("old", base)
	recent := pod("recent", base.Add(time.Minute))

	c, _ := newClient(t, dying, old, recent)
	handle(t, c, recent)

	got := costs(t, c, "dying", "old", "recent")
	assert.NotContains(t, got, "dying", "terminating pod must not be ranked")
	assert.Equal(t, 1, got["old"], "ranks must ignore the terminating pod")
	assert.Equal(t, 2, got["recent"])
}

func TestHandler_Handle_SkipsWhenTriggerIsDeleting(t *testing.T) {
	old := pod("old", base)
	dying := pod("dying", base.Add(time.Minute), deleting())

	c, patches := newClient(t, old, dying)
	handle(t, c, dying)

	assert.Zero(t, *patches, "a terminating trigger pod must not start a ranking pass")
	assert.Empty(t, costs(t, c, "old", "dying"))
}

// TestHandler_Handle_RanksStartAtOne matters because a pod without the
// annotation is read as cost 0: 1-based ranks keep every ranked pod above the
// not-yet-ranked ones.
func TestHandler_Handle_RanksStartAtOne(t *testing.T) {
	only := pod("only", base)

	c, _ := newClient(t, only)
	handle(t, c, only)

	assert.Equal(t, 1, costs(t, c, "only")["only"], "the oldest pod must rank at 1, not 0")
}

// TestHandler_Handle_IncludesNotReadyPods keeps ranks stable as pods become
// ready; Kubernetes already prefers unready pods for deletion before it looks
// at pod-deletion-cost.
func TestHandler_Handle_IncludesNotReadyPods(t *testing.T) {
	pending := pod("pending", base, notReady())
	ready := pod("ready", base.Add(time.Minute))

	c, _ := newClient(t, pending, ready)
	handle(t, c, ready)

	assert.Equal(t, map[string]int{"pending": 1, "ready": 2}, costs(t, c, "pending", "ready"))
}

func TestHandler_Handle_IgnoresPodsOfOtherReplicaSets(t *testing.T) {
	mine := pod("mine", base.Add(time.Minute))
	foreign := pod("foreign", base, withOwnerRS(otherRS))

	c, _ := newClient(t, mine, foreign)
	handle(t, c, mine)

	got := costs(t, c, "mine", "foreign")
	assert.Equal(t, 1, got["mine"], "ranking is scoped to the pod's own ReplicaSet")
	assert.NotContains(t, got, "foreign", "pods of another ReplicaSet must not be ranked")
}

func TestHandler_Handle_NoOwningReplicaSetIsNoOp(t *testing.T) {
	orphan := pod("orphan", base)
	orphan.OwnerReferences = nil

	c, patches := newClient(t, orphan)
	handle(t, c, orphan)

	assert.Zero(t, *patches)
	assert.Empty(t, costs(t, c, "orphan"))
}

// TestHandler_Handle_RecompactsRanksAfterEviction pins the observed behaviour
// that a scale-in shifts every survivor's rank: because a cost is a pod's
// position in the age order, removing older pods renumbers the rest from 1.
// Relative order is preserved, but it costs one patch per surviving pod.
func TestHandler_Handle_RecompactsRanksAfterEviction(t *testing.T) {
	// Survivors of an 11 -> 3 scale-in still carry their old ranks.
	a := pod("survivor-a", base.Add(7*time.Minute), withCost(9), withManagedBy("timestamp-rank"))
	b := pod("survivor-b", base.Add(8*time.Minute), withCost(10), withManagedBy("timestamp-rank"))
	c := pod("survivor-c", base.Add(9*time.Minute), withCost(11), withManagedBy("timestamp-rank"))

	cl, patches := newClient(t, a, b, c)
	handle(t, cl, c)

	got := costs(t, cl, "survivor-a", "survivor-b", "survivor-c")
	assert.Equal(t, map[string]int{"survivor-a": 1, "survivor-b": 2, "survivor-c": 3}, got,
		"survivors must be renumbered from 1 while keeping their relative order")
	assert.Equal(t, 3, *patches, "one patch per surviving pod")
}

// TestHandler_Handle_ScaleOutDoesNotRewriteExistingPods is the cheap counterpart:
// new pods are the youngest, so they take the ranks after the existing ones and
// no already-ranked pod is touched.
func TestHandler_Handle_ScaleOutDoesNotRewriteExistingPods(t *testing.T) {
	existing := pod("existing", base, withCost(1), withManagedBy("timestamp-rank"))
	fresh1 := pod("fresh-1", base.Add(time.Minute))
	fresh2 := pod("fresh-2", base.Add(time.Minute))

	cl, patches := newClient(t, existing, fresh1, fresh2)
	handle(t, cl, fresh1)

	got := costs(t, cl, "existing", "fresh-1", "fresh-2")
	assert.Equal(t, 1, got["existing"], "an already correct pod must not be renumbered")
	assert.Equal(t, map[string]int{"existing": 1, "fresh-1": 2, "fresh-2": 3}, got)
	assert.Equal(t, 2, *patches, "only the two new pods may be patched")
}

func TestHandler_Handle_PropagatesListError(t *testing.T) {
	boom := errors.New("index unavailable")
	c := fake.NewClientBuilder().
		WithScheme(newScheme(t)).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(_ context.Context, _ crclient.WithWatch, _ crclient.ObjectList, _ ...crclient.ListOption) error {
				return boom
			},
		}).
		Build()

	h := timestamprank.NewHandler(c)
	err := h.Handle(context.Background(), zap.New(zap.UseDevMode(true)), pod("p", base), enabledDeployment())

	require.ErrorIs(t, err, boom, "a failed listing must surface so the reconcile is retried")
}

func TestHandler_Handle_PropagatesPatchError(t *testing.T) {
	boom := errors.New("conflict")
	c, _ := newClientWithPatchError(t, boom, pod("p", base))

	h := timestamprank.NewHandler(c)
	err := h.Handle(context.Background(), zap.New(zap.UseDevMode(true)), pod("p", base), enabledDeployment())

	require.ErrorIs(t, err, boom, "a failed patch must surface so the reconcile is retried")
}
