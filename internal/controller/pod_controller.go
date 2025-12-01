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
	v2 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
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

func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := zone.CreatePodToOwnerRSIndex(mgr); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(cost.AcceptPodPredicateFunc()).
		Watches(&v1.ReplicaSet{},
			handler.EnqueueRequestForOwner(
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&v1.Deployment{},
				handler.OnlyControllerOwner(),
			),
		).
		Watches(&v1.Deployment{},
			handler.EnqueueRequestsFromMapFunc(r.mapDeploymentToPods),
			builder.WithPredicates(cost.AcceptDeploymentPredicateFunc()),
		).
		Watches(&corev1.Node{}, handler.Funcs{}).
		Complete(r)
}

func (r *PodReconciler) mapDeploymentToPods(ctx context.Context, obj client.Object) []reconcile.Request {
	dep, ok := obj.(*v1.Deployment)
	if !ok {
		return nil
	}
	podList := &corev1.PodList{}
	sel, err := v2.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return nil
	}

	if err := r.List(ctx, podList,
		client.InNamespace(dep.Namespace),
		client.MatchingLabelsSelector{Selector: sel},
	); err != nil {
		return nil
	}

	reqs := make([]reconcile.Request, 0, len(podList.Items))
	for _, pod := range podList.Items {
		reqs = append(reqs, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Namespace: pod.Namespace,
				Name:      pod.Name,
			},
		})
	}
	return reqs
}
