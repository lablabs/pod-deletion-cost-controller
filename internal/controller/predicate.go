package controller

import (
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func DeploymentPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		dep, ok := obj.(*v1.Deployment)
		if !ok {
			return false
		}
		return IsEnabled(dep)
	})
}

func PodPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return false
		}
		return IsAccepted(pod)
	})
}

func IsDeleting(pod *corev1.Pod) bool {
	return pod.DeletionTimestamp != nil
}

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
