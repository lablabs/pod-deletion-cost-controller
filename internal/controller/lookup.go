package controller

import (
	"context"
	"fmt"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	//PodToRSIndex index name for Pod to Rs
	PodToRSIndex = "spec.rsUID"
	// RsToDeploymentIndex index name for Rs to Deployment
	RsToDeploymentIndex = "spec.deploymentUID"
)

// createPodToRSIndex create index for mapping Pod to ReplicaSet owner reference UID
func createPodToRSIndex(mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Pod{}, PodToRSIndex, func(obj client.Object) []string {
		pod := obj.(*corev1.Pod)
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "ReplicaSet" {
				return []string{string(owner.UID)}
			}
		}
		return nil
	})
}

// createRsToDeploymentIndex create index for mapping ReplicaSet owner reference UID
func createRsToDeploymentIndex(mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(context.Background(), &v1.ReplicaSet{}, RsToDeploymentIndex, func(obj client.Object) []string {
		rs := obj.(*v1.ReplicaSet)
		for _, owner := range rs.OwnerReferences {
			if owner.Kind == "Deployment" {
				return []string{string(owner.UID)}
			}
		}
		return nil
	})
}

func mapDeploymentToPodReconcileFunc(c client.Client) handler.MapFunc {
	return func(ctx context.Context, object client.Object) []reconcile.Request {
		dep := object.(*v1.Deployment)
		if !IsEnabled(dep) {
			return nil
		}
		rsList := &v1.ReplicaSetList{}
		if err := c.List(ctx, rsList, client.MatchingFields{RsToDeploymentIndex: string(dep.UID)}); err != nil {
			return nil
		}

		reqs := make([]reconcile.Request, 0)
		for _, rs := range rsList.Items {
			podList := &corev1.PodList{}
			if err := c.List(ctx, podList, client.MatchingFields{PodToRSIndex: string(rs.UID)}); err != nil {
				continue
			}
			for _, pod := range podList.Items {
				if !IsAccepted(&pod) {
					continue
				}
				if HasPodDeletionCost(&pod) {
					continue
				}
				reqs = append(reqs, reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name},
				})
			}
		}

		return reqs
	}
}

// GetDeployment return deployment associated with Pod
func GetDeployment(ctx context.Context, c client.Client, pod *corev1.Pod) (*v1.Deployment, error) {
	var rsName string
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "ReplicaSet" {
			rsName = owner.Name
			break
		}
	}
	if rsName == "" {
		return nil, fmt.Errorf("pod %s/%s has no owning ReplicaSet", pod.Namespace, pod.Name)
	}

	rs := &v1.ReplicaSet{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: rsName}, rs); err != nil {
		return nil, fmt.Errorf("get replicaset %s/%s: %w", pod.Namespace, rsName, err)
	}

	// 2) Find owning Deployment
	var deployName string
	for _, owner := range rs.OwnerReferences {
		if owner.Kind == "Deployment" {
			deployName = owner.Name
			break
		}
	}
	if deployName == "" {
		return nil, fmt.Errorf("replicaset %s/%s has no owning Deployment", rs.Namespace, rs.Name)
	}

	deploy := &v1.Deployment{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: rs.Namespace, Name: deployName}, deploy); err != nil {
		return nil, fmt.Errorf("get deployment %s/%s: %w", rs.Namespace, deployName, err)
	}

	return deploy, nil
}
