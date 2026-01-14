package controller

import (
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	// PodDeletionCostAnnotation pod-deletion-cost annotation used by k8s to determine Pod for deletion
	PodDeletionCostAnnotation = "controller.kubernetes.io/pod-deletion-cost"
	// EnableAnnotation Use for enable pod-deletion-cost on Deployment. Default annotation used for distribution of cost deletion
	// annotation is 'topology.kubernetes.io/zone'. Can be overridden by SpreadByAnnotation
	EnableAnnotation = "pod-deletion-cost.lablabs.io/enabled"
	// TypeAnnotation can be used to specify algorithm used for pod-deletion-cost selection
	TypeAnnotation = "pod-deletion-cost.lablabs.io/type"
)

// ApplyPodDeletionCost apply PodDeletionCostAnnotation to Pod wit value
func ApplyPodDeletionCost(pod *corev1.Pod, value int) {
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[PodDeletionCostAnnotation] = strconv.Itoa(value)
}

// GetPodDeletionCost get PodDeletionCostAnnotation
func GetPodDeletionCost(pod *corev1.Pod) (int, bool) {
	if pod.Annotations == nil {
		return 0, false
	}
	v, ok := pod.Annotations[PodDeletionCostAnnotation]
	if !ok {
		return 0, false
	}
	value, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return value, true
}

// IsEnabled return true id Deployment has EnableAnnotation enabled
func IsEnabled(dep *appsv1.Deployment) bool {
	if dep.Annotations == nil {
		return false
	}
	return dep.Annotations[EnableAnnotation] == "true"
}

// GetType return TypeAnnotation
func GetType(dep *appsv1.Deployment) string {
	if dep.Annotations == nil {
		return ""
	}
	return dep.Annotations[TypeAnnotation]
}

// HasPodDeletionCost checks if Pod has PodDeletionCostAnnotation
func HasPodDeletionCost(pod *corev1.Pod) bool {
	if pod.Annotations == nil {
		return false
	}
	_, ok := pod.Annotations[PodDeletionCostAnnotation]
	return ok
}
