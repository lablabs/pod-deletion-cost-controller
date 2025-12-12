package zone

import (
	v2 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

const (
	//TopologyZoneAnnotation Default annotation used by Kubernetes
	TopologyZoneAnnotation = "topology.kubernetes.io/zone"
	//SpreadByAnnotation overrides name of annotation used for spreading pod-deletion-cost logic
	SpreadByAnnotation = "pod-deletion-cost.lablabs.io/spread-by"
)

func GetSpreadByAnnotation(node *v1.Node, deployment *v2.Deployment) string {
	if deployment.Annotations == nil {
		return node.Labels[TopologyZoneAnnotation]
	}
	if spreadBy, ok := deployment.Annotations[SpreadByAnnotation]; ok {
		return node.Labels[spreadBy]
	}
	return node.Labels[TopologyZoneAnnotation]
}
