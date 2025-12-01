package zone

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/lablabs/pod-deletion-cost-controller/internal/cost"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	PodToOwnerRSIndex = "rsOwnerUID"
)

func NewHandler(client client.Client, option ...Option) (*Handler, error) {

	cfg := config{
		spreadByAnnotation: TopologyZoneAnnotation,
	}
	for _, opt := range option {
		if err := opt(&cfg); err != nil {
			return nil, fmt.Errorf("option: %w", err)
		}
	}
	h := Handler{
		Client: client,
		cfg:    cfg,
	}
	return &h, nil
}

type Handler struct {
	client.Client
	cfg config
}

func (h *Handler) Process(ctx context.Context, log logr.Logger, pod *corev1.Pod, deployment *v1.Deployment) error {

	pods := make([]corev1.Pod, 0)
	err := h.ListPodsInZone(ctx, log, deployment, pod, &pods)
	if err != nil {
		return fmt.Errorf("unable to list pods: %w", err)
	}
	costAll := NewDeletionCostList(pods)
	c, err := costAll.FindNext()
	if err != nil {
		return fmt.Errorf("unable to find next cost value: %w", err)
	}
	ApplyPodDeletionCost(pod, c)
	if err := h.Update(ctx, pod); err != nil {
		return err
	}
	log.WithValues(cost.PodDeletionCostAnnotation, c).Info("applied")
	return nil
}

func (h *Handler) ListPodsInZone(
	ctx context.Context,
	log logr.Logger,
	deployment *v1.Deployment,
	pod *corev1.Pod,
	pods *[]corev1.Pod,
) error {

	podRecZoneAnn, err := h.GetPodAnnotation(ctx, pod, deployment)
	if err != nil {
		return fmt.Errorf("unable to get pod annotation: %w", err)
	}
	podList := &corev1.PodList{}
	err = ListPodsByOwnerRSIndex(ctx, h, pod, podList)
	if err != nil {
		return fmt.Errorf("unable to list pods by rs: %w", err)
	}

	node := &corev1.Node{}
	for _, pod := range podList.Items {
		if err := h.Get(ctx, types.NamespacedName{Name: pod.Spec.NodeName}, node); err != nil {
			return err
		}
		zoneAnn := GetSpreadByAnnotationValue(node, deployment)
		if podRecZoneAnn != zoneAnn {
			continue
		}
		*pods = append(*pods, pod)
	}
	log.WithValues("node", pod.Spec.NodeName, "zone", podRecZoneAnn, "zone-pod-count", len(*pods))
	return nil
}

func (h *Handler) GetPodAnnotation(ctx context.Context, pod *corev1.Pod, deployment *v1.Deployment) (string, error) {
	//Get zone for reconcile POD
	node := &corev1.Node{}
	if err := h.Get(ctx, types.NamespacedName{
		Name: pod.Spec.NodeName,
	}, node); err != nil {
		return "", err
	}
	return GetSpreadByAnnotationValue(node, deployment), nil
}

func CreatePodToOwnerRSIndex(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(),
		&corev1.Pod{}, PodToOwnerRSIndex,
		func(obj client.Object) []string {
			pod := obj.(*corev1.Pod)
			for _, owner := range pod.OwnerReferences {
				if owner.Kind == "ReplicaSet" {
					return []string{string(owner.UID)}
				}
			}
			return nil
		},
	); err != nil {
		return err
	}
	return nil
}

func ListPodsByOwnerRSIndex(ctx context.Context, c client.Client, pod *corev1.Pod, list *corev1.PodList) error {
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
		client.MatchingFields{PodToOwnerRSIndex: string(rsUID)},
	)
	if err != nil {
		return err
	}
	return nil
}
