# pod-deletion-cost-controller

Controller for injecting `controller.kubernetes.io/pod-deletion-cost` annotation into running Pod. This annotation influences
which Pods are terminated first during downscaling. This allows the controller to make smarter decisions during scale-down and
avoid removing too many Pods from a single availability zone, which could compromise resilience.
In practice, by defining a deletion cost strategy across Pods, Kubernetes can evenly distribute termination events and maintain
high availability even under reduction of replicas. This results in balanced Pod placement across zones and a safer,
more predictable downscaling behavior. For more information, follow context below

- [replicaset/#pod-deletion-cost](https://kubernetes.io/docs/concepts/workloads/controllers/replicaset/#pod-deletion-cost)
- [k8s-algorithm-pick-pod-scale-in](https://rpadovani.com/k8s-algorithm-pick-pod-scale-in)
- [descheduler](https://github.com/kubernetes-sigs/descheduler?tab=readme-ov-file#removepodsviolatingtopologyspreadconstraint)
- [keps/sig-apps/2255-pod-cost](https://github.com/kubernetes/enhancements/tree/master/keps/sig-apps/2255-pod-cost)

# Usage & Configuration

Enable on k8s/Deployment via `pod-deletion-cost.lablabs.io/enabled: "true"` annotation:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  labels:
    app: nginx
  annotations:
    pod-deletion-cost.lablabs.io/enabled: "true"
spec:
  replicas: 8
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: ScheduleAnyway
          labelSelector:
            matchLabels:
              app: nginx
      containers:
        - name: nginx
          image: nginx:latest
```

In default configuration, controller looks for node's label `topology.kubernetes.io/zone` and base on value
evenly distribute termination order via `controller.kubernetes.io/pod-deletion-cost`. In case Nodes are deployed
in no zone env, it can be overridden via `pod-deletion-cost.lablabs.io/spread-by` which define Label name of Node
used for spread topology counting logic.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  labels:
    app: nginx
  annotations:
    pod-deletion-cost.lablabs.io/enabled: "true"
    pod-deletion-cost.lablabs.io/spread-by: "topology.kubernetes.io/rack" # Example of annotation, on-prem etc..
spec:
  replicas: 8
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/rack
          whenUnsatisfiable: ScheduleAnyway
          labelSelector:
            matchLabels:
              app: nginx
      containers:
        - name: nginx
          image: nginx:latest
```

# Install

Helm install:
```bash
VERSION=v0.0.0-alpha.2
helm pull oci://ghcr.io/lablabs/pod-deletion-cost-controller/pod-deletion-cost-controller --version ${VERSION}
helm upgrade --install -n operations \
    --create-namespace pod-deletion-cost-controller  \
    oci://ghcr.io/lablabs/pod-deletion-cost-controller/pod-deletion-cost-controller \
    --version ${VERSION}
```

# Development

Build with [kubebuilder](https://book.kubebuilder.io)

Dependencies:

- [Kind](https://kind.sigs.k8s.io/) - for running tests. It's not downloaded automatically as the other tools in bin folder

# Test

```bash
# Run unit tests
make test
# Run e2e tests with kind
make test-e2e
```