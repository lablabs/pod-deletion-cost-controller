package zone

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	// TopologyZoneAnnotation Default annotation used by Kubernetes
	TopologyZoneAnnotation = "topology.kubernetes.io/zone"
	// SpreadByAnnotation overrides name of annotation used for spreading pod-deletion-cost logic
	SpreadByAnnotation = "pod-deletion-cost.lablabs.io/spread-by"
)

// GetSpreadByAnnotation get SpreadByAnnotation annotation
func GetSpreadByAnnotation(node *corev1.Node, deployment *appsv1.Deployment) string {
	if deployment == nil {
		return ""
	}
	if node == nil {
		return ""
	}
	if deployment.Annotations == nil {
		return node.Labels[TopologyZoneAnnotation]
	}
	if spreadBy, ok := deployment.Annotations[SpreadByAnnotation]; ok {
		return node.Labels[spreadBy]
	}
	return node.Labels[TopologyZoneAnnotation]
}
