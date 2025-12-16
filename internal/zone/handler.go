package zone

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/lablabs/pod-deletion-cost-controller/internal/controller"
	"github.com/lablabs/pod-deletion-cost-controller/internal/expectations"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// TypeAnnotation name of algo type
	TypeAnnotation = "zone"
	// DefaultHandlerTypeAnnotation define default, it means, alg type is not defined
	DefaultHandlerTypeAnnotation = ""
)

// NewHandler create new Handler
func NewHandler(client client.Client) *Handler {
	return &Handler{
		client: client,
		cache:  expectations.NewCache[types.UID, int](),
	}
}

// Handler handles reconcile loop for Pod/Deployment
type Handler struct {
	client client.Client
	cache  *expectations.Cache[types.UID, int]
}

// AcceptType return accepted type of reconcile algorithm
func (h *Handler) AcceptType() []string {
	return []string{TypeAnnotation, DefaultHandlerTypeAnnotation}
}

// Handle handles main Reconcile for zone
func (h *Handler) Handle(ctx context.Context, log logr.Logger, pod *corev1.Pod, dep *v1.Deployment) error {

	if controller.HasPodDeletionCost(pod) {
		h.cache.Delete(pod.UID)
		log.V(3).Info("clean cache, pod was sync")
		return nil
	}

	if controller.IsDeleting(pod) {
		return nil
	}

	pods := make([]corev1.Pod, 0)
	err := h.listPodsInZone(ctx, log, dep, pod, &pods)
	if err != nil {
		return fmt.Errorf("unable to list pods: %w", err)
	}

	pool := NewDeletionCostPool()
	for _, pod := range pods {
		if cost, exist := controller.GetPodDeletionCost(&pod); exist {
			pool.AddValue(cost)
			continue
		}
		if v, cached := h.cache.Get(pod.UID); cached {
			pool.AddValue(v)
		}
	}
	cost, err := pool.FindNextFree()
	if err != nil {
		return fmt.Errorf("unable to find next cost value: %w", err)
	}
	h.cache.Set(pod.UID, cost)

	patch := client.MergeFrom(pod.DeepCopy())
	controller.ApplyPodDeletionCost(pod, cost)
	err = h.client.Patch(ctx, pod, patch)
	if err != nil {
		return err
	}
	log.WithValues(controller.PodDeletionCostAnnotation, cost).Info("updated")
	return nil
}

func (h *Handler) listPodsInZone(
	ctx context.Context,
	log logr.Logger,
	deployment *v1.Deployment,
	pod *corev1.Pod,
	pods *[]corev1.Pod,
) error {
	podRecZoneAnn, err := h.getPodAnnotation(ctx, pod, deployment)
	if err != nil {
		return fmt.Errorf("unable to get pod annotation: %w", err)
	}
	podList := &corev1.PodList{}
	err = listPodsByOwnerRSIndex(ctx, h.client, pod, podList)
	if err != nil {
		return fmt.Errorf("unable to list pods by rs: %w", err)
	}

	node := &corev1.Node{}
	for _, pod := range podList.Items {
		if err := h.client.Get(ctx, types.NamespacedName{Name: pod.Spec.NodeName}, node); err != nil {
			return err
		}
		zoneAnn := GetSpreadByAnnotation(node, deployment)
		if podRecZoneAnn != zoneAnn {
			continue
		}
		*pods = append(*pods, pod)
	}
	log.WithValues("node", pod.Spec.NodeName, "zone", podRecZoneAnn, "zone-pod-count", len(*pods))
	return nil
}

func (h *Handler) getPodAnnotation(ctx context.Context, pod *corev1.Pod, deployment *v1.Deployment) (string, error) {
	//Get zone for reconcile POD
	node := &corev1.Node{}
	if err := h.client.Get(ctx, types.NamespacedName{
		Name: pod.Spec.NodeName,
	}, node); err != nil {
		return "", err
	}
	return GetSpreadByAnnotation(node, deployment), nil
}

func listPodsByOwnerRSIndex(ctx context.Context, c client.Client, pod *corev1.Pod, list *corev1.PodList) error {
	var rsUID types.UID
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			rsUID = owner.UID
			break
		}
	}

	if rsUID == "" {
		return nil
	}

	err := c.List(ctx, list,
		client.InNamespace(pod.Namespace),
		client.MatchingFields{controller.PodToRSIndex: string(rsUID)},
	)
	if err != nil {
		return err
	}
	return nil
}
