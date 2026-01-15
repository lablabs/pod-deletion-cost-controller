# Pod Deletion Cost Controller

A Kubernetes controller that automatically manages the [`controller.kubernetes.io/pod-deletion-cost`](https://kubernetes.io/docs/concepts/workloads/controllers/replicaset/#pod-deletion-cost) annotation on pods. This annotation influences which pods are terminated first during scale-down operations, enabling smarter and more resilient downscaling behavior.

The controller is designed to be **extensible** with a plugin-based architecture, allowing multiple algorithms for calculating pod deletion costs. Currently, it includes a **zone-aware distribution algorithm** that ensures even pod termination across availability zones.

## The Problem: Default Kubernetes Scale-Down Behavior

When Kubernetes scales down a Deployment or ReplicaSet, it uses a [specific algorithm](https://github.com/kubernetes/kubernetes/blob/release-1.32/pkg/controller/replicaset/replica_set.go#L836) to determine which pods to delete first. The default selection criteria are:

1. Pods that are unassigned (not scheduled to a node)
2. Pods in Pending or Unknown phase
3. Pods not ready vs. pods ready
4. Pods with lower pod-deletion-cost annotation value
5. Pods with more recent creation timestamps
6. Random selection as a tiebreaker

**The issue:** Without explicit `pod-deletion-cost` annotations, Kubernetes may delete pods unevenly across availability zones during scale-down. This can lead to:

- **Imbalanced zone distribution** - One zone might lose significantly more pods than others
- **Reduced resilience** - Workloads become vulnerable to zone failures
- **Topology constraint violations** - Even with `topologySpreadConstraints`, scale-down doesn't respect zone distribution

For example, if you have 6 pods spread across 3 zones (2 per zone) and scale down to 3 pods, Kubernetes might delete 2 pods from Zone A and 1 from Zone B, leaving you with an unbalanced distribution (0 in Zone A, 1 in Zone B, 2 in Zone C).

## Solution: Pod Deletion Cost Annotation

Kubernetes provides the [`controller.kubernetes.io/pod-deletion-cost`](https://kubernetes.io/docs/concepts/workloads/controllers/replicaset/#pod-deletion-cost) annotation (stable since v1.22) to influence pod deletion order:

- **Lower values** = Higher deletion priority (deleted first)
- **Higher values** = Lower deletion priority (deleted last)
- Valid range: -2147483648 to 2147483647

This controller automatically assigns these values based on configurable algorithms, ensuring predictable and resilient scale-down behavior.

### References

- [Kubernetes ReplicaSet - Pod Deletion Cost](https://kubernetes.io/docs/concepts/workloads/controllers/replicaset/#pod-deletion-cost)
- [KEP-2255: Pod Deletion Cost](https://github.com/kubernetes/enhancements/tree/master/keps/sig-apps/2255-pod-cost)
- [Understanding K8s Scale-In Algorithm](https://rpadovani.com/k8s-algorithm-pick-pod-scale-in)
- [Descheduler - TopologySpreadConstraint](https://github.com/kubernetes-sigs/descheduler?tab=readme-ov-file#removepodsviolatingtopologyspreadconstraint)

## Current Algorithm: Zone-Aware Distribution

The `zone` algorithm (default) ensures even pod distribution across availability zones during scale-down.

### How It Works

1. **Pod Detection** - Controller watches for pods belonging to enabled Deployments
2. **Zone Identification** - Determines the pod's zone from its node's `topology.kubernetes.io/zone` label
3. **Cost Calculation** - Assigns unique deletion costs within each zone, starting from MaxInt32 (2147483647) and descending
4. **Annotation** - Applies `controller.kubernetes.io/pod-deletion-cost` to the pod

### Algorithm Details

Within each zone, pods receive descending cost values:
- First pod in zone: `2147483647` (most protected)
- Second pod in zone: `2147483646`
- And so on...

Different zones independently allocate their own cost values. This ensures that during scale-down, Kubernetes removes pods evenly across zones.

### Example Scenario

**Initial state:** 6 pods across 3 zones

```
Zone A: Pod1 (cost: 2147483647), Pod2 (cost: 2147483646)
Zone B: Pod3 (cost: 2147483647), Pod4 (cost: 2147483646)
Zone C: Pod5 (cost: 2147483647), Pod6 (cost: 2147483646)
```

**After scaling down to 3 pods:**

Kubernetes deletes pods with the lowest costs first. Since each zone has pods with cost `2147483646`, one pod is removed from each zone:

```
Zone A: Pod1 (cost: 2147483647)
Zone B: Pod3 (cost: 2147483647)
Zone C: Pod5 (cost: 2147483647)
```

Result: **Even distribution maintained** across all zones.

![Pod Deletion Cost Controller Flow](./docs/images/pod-deletion-cost-controller-flow.gif)

## Installation

### Helm

```bash
VERSION=v0.0.0-alpha.2

# Pull the chart (optional)
helm pull oci://ghcr.io/lablabs/pod-deletion-cost-controller/pod-deletion-cost-controller \
  --version ${VERSION}

# Install
helm upgrade --install pod-deletion-cost-controller \
  oci://ghcr.io/lablabs/pod-deletion-cost-controller/pod-deletion-cost-controller \
  --namespace operations \
  --create-namespace \
  --version ${VERSION}
```

### Helm Values

Key configuration options in `values.yaml`:

```yaml
# Algorithms to enable
algorithms:
  - "zone"

# Logging configuration
log:
  devel: false
  encoder: console  # or "json"
  level: 3          # 0=debug, 1=info, 2=warn, 3=error

# Health probes
health:
  enabled: true
  port: 8001

# Metrics
metrics:
  enabled: true
  service:
    ports:
      metrics: 9000

# High availability
replicaCount: 1
pdb:
  enabled: false
  maxUnavailable: 1
```

## Usage

### Enable for a Deployment

Add the `pod-deletion-cost.lablabs.io/enabled: "true"` annotation to your Deployment:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    pod-deletion-cost.lablabs.io/enabled: "true"
spec:
  replicas: 6
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: ScheduleAnyway
          labelSelector:
            matchLabels:
              app: my-app
      containers:
        - name: my-app
          image: my-app:latest
```

### Configuration Annotations

| Annotation | Required | Default | Description |
|-----------|----------|---------|-------------|
| `pod-deletion-cost.lablabs.io/enabled` | Yes | - | Set to `"true"` to enable the controller |
| `pod-deletion-cost.lablabs.io/type` | No | `zone` | Algorithm type to use |
| `pod-deletion-cost.lablabs.io/spread-by` | No | `topology.kubernetes.io/zone` | Node label key for topology spreading |

### Custom Topology Label

For on-premises or custom environments, you can specify a different node label for topology spreading:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    pod-deletion-cost.lablabs.io/enabled: "true"
    pod-deletion-cost.lablabs.io/spread-by: "topology.kubernetes.io/rack"
spec:
  replicas: 6
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/rack
          whenUnsatisfiable: ScheduleAnyway
          labelSelector:
            matchLabels:
              app: my-app
      containers:
        - name: my-app
          image: my-app:latest
```

### Explicit Algorithm Selection

While `zone` is the default algorithm, you can explicitly specify it:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-app
  annotations:
    pod-deletion-cost.lablabs.io/enabled: "true"
    pod-deletion-cost.lablabs.io/type: "zone"
```

## Contributing

The controller uses an extensible plugin-based architecture, making it easy to add new algorithms for different use cases. We welcome contributions!

See [CONTRIBUTING.md](./CONTRIBUTING.md) for:

- Architecture overview and key components
- Step-by-step guide for adding new algorithms
- Development setup and build instructions
- Testing requirements
- Code style guidelines

## License

Apache License 2.0 - see [LICENSE](./LICENSE) for details.
