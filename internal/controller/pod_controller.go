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
	deploy, err := zone.GetDeploymentByPod(ctx, r.Client, pod)
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
		dep, err := zone.GetDeploymentByPod(context.Background(), c, pod)
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
	rsList := &v1.ReplicaSetList{}
	if err := r.List(ctx, rsList, client.MatchingFields{zone.RsToDeploymentIndex: string(dep.UID)}); err != nil {
		return nil
	}

	reqs := make([]reconcile.Request, 0)
	for _, rs := range rsList.Items {
		podList := &corev1.PodList{}
		if err := r.List(ctx, podList, client.MatchingFields{zone.PodToRSIndex: string(rs.UID)}); err != nil {
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
	if err := zone.CreatePodToRSIndex(mgr); err != nil {
		return err
	}
	if err := zone.CreateRsToDeploymentIndex(mgr); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}, builder.WithPredicates(AcceptPodPredicate(r.Client))).
		Watches(&v1.ReplicaSet{}, handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &v1.Deployment{}, handler.OnlyControllerOwner())).
		Watches(&v1.Deployment{}, handler.EnqueueRequestsFromMapFunc(r.mapDeploymentToPods), builder.WithPredicates(AcceptDeploymentPredicate())).
		Watches(&corev1.Node{}, handler.Funcs{}).
		Complete(r)
}
