package cost

import (
	"context"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func AcceptDeploymentPredicateFunc() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		dep, ok := obj.(*v1.Deployment)
		if !ok {
			return false
		}
		if dep.Annotations == nil {
			return false
		}
		return dep.Annotations[EnableAnnotation] == "true"
	})
}

func AcceptPodPredicateFunc(c client.Client) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return false
		}

		// 1. Only process pods with Deployment owner
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "ReplicaSet" {
				rs := &v1.ReplicaSet{}
				if err := c.Get(context.Background(),
					client.ObjectKey{Name: owner.Name, Namespace: pod.Namespace},
					rs,
				); err != nil {
					return false
				}

				for _, owner2 := range rs.OwnerReferences {
					if owner2.Kind == "Deployment" {
						dep := &v1.Deployment{}
						if err := c.Get(context.Background(),
							client.ObjectKey{Name: owner2.Name, Namespace: pod.Namespace},
							dep,
						); err != nil {
							return false
						}

						// Check the annotation!
						if dep.Annotations[EnableAnnotation] != "true" {
							return false
						}
					}
				}
			}
		}

		if pod.Spec.NodeName == "" {
			return false
		}
		if pod.Status.Phase != corev1.PodRunning {
			return false
		}
		isReady := false
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				isReady = true
				break
			}
		}
		if !isReady {
			return false
		}
		if _, ok := pod.Annotations[PodDeletionCostAnnotation]; ok {
			return false
		}
		return true
	})
}
