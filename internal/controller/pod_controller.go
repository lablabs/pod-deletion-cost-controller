/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	"github.com/lablabs/pod-deletion-cost-controller/internal/cost"
	"github.com/lablabs/pod-deletion-cost-controller/internal/zone"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	Handler *zone.Handler
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=pods/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Pod object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
// Reconcile is called for each Pod event

func (r *PodReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	pod := &corev1.Pod{}
	if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if cost.IsPodDeletionCostAnnotation(pod) {
		return ctrl.Result{}, nil
	}
	deploy, err := r.getDeployment(ctx, pod)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if deploy == nil {
		return ctrl.Result{}, nil
	}
	err = r.Handler.Process(ctx, logger, pod, deploy)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	return ctrl.Result{}, nil
}

func (r *PodReconciler) getDeployment(ctx context.Context, pod *corev1.Pod) (*v1.Deployment, error) {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "ReplicaSet" && *owner.Controller {
			rs := &v1.ReplicaSet{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: owner.Name}, rs); err != nil {
				return nil, client.IgnoreNotFound(err)
			}

			for _, rsOwner := range rs.OwnerReferences {
				if rsOwner.Kind == "Deployment" && *rsOwner.Controller {
					dep := &v1.Deployment{}
					if err := r.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: rsOwner.Name}, dep); err != nil {
						return nil, client.IgnoreNotFound(err)
					}
					return dep, nil
				}
			}
		}
	}
	return nil, nil
}

func CreatePodToDeploymentIndex(mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Pod{}, "spec.deploymentName", func(o client.Object) []string {
		pod := o.(*corev1.Pod)
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "ReplicaSet" {
				rs := &v1.ReplicaSet{}
				if err := mgr.GetClient().Get(context.Background(), client.ObjectKey{Namespace: pod.Namespace, Name: owner.Name}, rs); err == nil {
					for _, owner2 := range rs.OwnerReferences {
						if owner2.Kind == "Deployment" {
							return []string{owner2.Name}
						}
					}
				}
			}
		}
		return nil
	})
}

func AcceptDeploymentPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		dep := obj.(*v1.Deployment)
		return cost.IsEnableAnnotation(dep)
	})
}

func AcceptPodPredicate(c client.Client) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		pod := obj.(*corev1.Pod)
		if pod.Status.Phase != corev1.PodRunning {
			return false
		}
		isReady := false
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				isReady = true
				break
			}
		}
		if !isReady {
			return false
		}
		if cost.IsPodDeletionCostAnnotation(pod) {
			return false
		}
		dep, err := GetDeploymentByPod(context.Background(), c, pod)
		if err != nil {
			return false
		}
		return cost.IsEnableAnnotation(dep)
	})
}

func (r *PodReconciler) mapDeploymentToPods(ctx context.Context, obj client.Object) []reconcile.Request {
	dep := obj.(*v1.Deployment)

	if !cost.IsEnableAnnotation(dep) {
		return nil
	}

	// 1) Get all ReplicaSets for this Deployment
	rsList := &v1.ReplicaSetList{}
	if err := r.List(ctx, rsList, client.MatchingFields{"spec.deploymentUID": string(dep.UID)}); err != nil {
		return nil
	}

	reqs := make([]reconcile.Request, 0)
	// 2) For each ReplicaSet, get all Pods using Pod->RS index
	for _, rs := range rsList.Items {
		podList := &corev1.PodList{}
		if err := r.List(ctx, podList, client.MatchingFields{"spec.rsUID": string(rs.UID)}); err != nil {
			continue
		}
		for _, pod := range podList.Items {
			if cost.IsPodDeletionCostAnnotation(&pod) {
				continue
			}
			reqs = append(reqs, reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name},
			})
		}
	}

	return reqs
}

func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := CreatePodToDeploymentIndex(mgr); err != nil {
		return err
	}
	if err := zone.CreatePodToOwnerRSIndex(mgr); err != nil {
		return err
	}
	if err := CreatePodToRSIndex(mgr); err != nil {
		return err
	}
	if err := CreateRsToDeploymentIndex(mgr); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}, builder.WithPredicates(AcceptPodPredicate(r.Client))).
		Watches(&v1.ReplicaSet{}, handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &v1.Deployment{}, handler.OnlyControllerOwner())).
		Watches(&v1.Deployment{}, handler.EnqueueRequestsFromMapFunc(r.mapDeploymentToPods), builder.WithPredicates(AcceptDeploymentPredicate())).
		Watches(&corev1.Node{}, handler.Funcs{}).
		Complete(r)
}

func CreatePodToRSIndex(mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.Pod{}, "spec.rsUID", func(obj client.Object) []string {
		pod := obj.(*corev1.Pod)
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "ReplicaSet" {
				return []string{string(owner.UID)}
			}
		}
		return nil
	})
}

func CreateRsToDeploymentIndex(mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(context.Background(), &v1.ReplicaSet{}, "spec.deploymentUID", func(obj client.Object) []string {
		rs := obj.(*v1.ReplicaSet)
		for _, owner := range rs.OwnerReferences {
			if owner.Kind == "Deployment" {
				return []string{string(owner.UID)}
			}
		}
		return nil
	})
}

func GetDeploymentByPod(ctx context.Context, c client.Client, pod *corev1.Pod) (*v1.Deployment, error) {
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
