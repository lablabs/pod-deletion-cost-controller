package controller

import (
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// DeploymentPredicate creates Deployment predicate for filtering
func DeploymentPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		dep, ok := obj.(*v1.Deployment)
		if !ok {
			return false
		}
		return IsEnabled(dep)
	})
}

// PodPredicate creates Pod predicate for filtering
func PodPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return false
		}
		return IsAccepted(pod)
	})
}

// IsDeleting return true if Pod is in deleting state
func IsDeleting(pod *corev1.Pod) bool {
	return pod.DeletionTimestamp != nil
}

// IsAccepted return true if Pod should be accepted by reconcile loop
func IsAccepted(pod *corev1.Pod) bool {
	//
	// 1️⃣ Always accept pods that are being deleted
	//
	if IsDeleting(pod) {
		return true
	}

	//
	// 2️⃣ Accept only running AND ready pods
	//
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}

	ready := false
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			ready = true
			break
		}
	}

	return ready
}
