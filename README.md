# pod-deletion-cost-controller

Controller for applying `controller.kubernetes.io/pod-deletion-cost` annotation. This annotation influences 
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

This annotation applies `controller.kubernetes.io/pod-deletion-cost` to Pods in a Deployment.
It helps ensure Pods are terminated evenly during downscaling. By default, Kubernetes does not 
distribute this across zones automatically — this chart adds that capability, using `topology.kubernetes.io/zone` 
unless overridden via `pod-deletion-cost.lablabs.io/spread-by`.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  labels:
    app: nginx
  annotations:
    pod-deletion-cost.lablabs.io/enabled: "true"
    pod-deletion-cost.lablabs.io/spread-by: "topology.kubernetes.io/rack" # Use this annotation instead of zone
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

Via helm chart


# Development

Build with [kubebuilder](https://book.kubebuilder.io)

Dependencies

- [Kind](https://kind.sigs.k8s.io/) - for running tests

# Test

```bash
# Run unit tests
make test 
# Run e2e tests with kind
make test-e2e
```


