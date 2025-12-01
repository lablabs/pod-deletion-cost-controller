package zone

import (
	"github.com/lablabs/pod-deletion-cost-controller/internal/cost"
	v2 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

const (
	//TopologyZoneAnnotation Default annotation used by Kubernetes
	TopologyZoneAnnotation = "topology.kubernetes.io/zone"
)

func GetSpreadByAnnotationValue(node *v1.Node, deployment *v2.Deployment) string {
	if deployment.Annotations == nil {
		return node.Labels[TopologyZoneAnnotation]
	}
	if spreadBy, ok := deployment.Annotations[cost.SpreadByAnnotation]; ok {
		return node.Labels[spreadBy]
	}
	return node.Labels[TopologyZoneAnnotation]
}
