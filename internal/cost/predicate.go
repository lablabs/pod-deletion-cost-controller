package cost

import (
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
		return ok && dep.Annotations[EnableAnnotation] == "true"
	})
}

func AcceptPodPredicateFunc() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return false
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
