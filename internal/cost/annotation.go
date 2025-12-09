package cost

import (
	v1 "k8s.io/api/apps/v1"
	v2 "k8s.io/api/core/v1"
)

const (
	PodDeletionCostAnnotation = "controller.kubernetes.io/pod-deletion-cost"
	// EnableAnnotation Use for enable pod-deletion-cost on Deployment. Default annotation used for distribution of cost deletion
	// annotation is 'topology.kubernetes.io/zone'. Can be overridden by SpreadByAnnotation
	EnableAnnotation = "pod-deletion-cost.lablabs.io/enabled"
	// SpreadByAnnotation overrides name of annotation used for spreading pod-deletion-cost logic
	SpreadByAnnotation = "pod-deletion-cost.lablabs.io/spread-by"
)

func IsEnableAnnotation(dep *v1.Deployment) bool {
	if dep.Annotations == nil {
		return false
	}
	return dep.Annotations[EnableAnnotation] == "true"
}

func IsPodDeletionCostAnnotation(pod *v2.Pod) bool {
	if pod.Annotations == nil {
		return false
	}
	_, ok := pod.Annotations[PodDeletionCostAnnotation]
	return ok
}
