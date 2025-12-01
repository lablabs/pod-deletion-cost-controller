package cost

const (
	PodDeletionCostAnnotation = "controller.kubernetes.io/pod-deletion-cost"
	// EnableAnnotation Use for enable pod-deletion-cost on Deployment. Default annotation used for distribution of cost deletion
	// annotation is 'topology.kubernetes.io/zone'. Can be overridden by SpreadByAnnotation
	EnableAnnotation = "pod-deletion-cost.lablabs.io/enabled"
	// SpreadByAnnotation overrides name of annotation used for spreading pod-deletion-cost logic
	SpreadByAnnotation = "pod-deletion-cost.lablabs.io/spread-by"
)
