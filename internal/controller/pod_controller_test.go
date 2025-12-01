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
	"github.com/lablabs/pod-deletion-cost-controller/test/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("PodDeletionCost Controller", func() {
	var (
		ctx = context.Background()
	)
	Context("when a Deployment with deletion-cost enabled creates a ReplicaSet and Pod", func() {
		It("adds the controller.kubernetes.io/pod-deletion-cost annotation to the Pod", func() {
			By("Creating Deployment, RS, Pods")
			deploy := &appsv1.Deployment{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-deploy",
					Namespace: "default",
					Labels:    map[string]string{"app": "nginx"},
					Annotations: map[string]string{
						cost.EnableAnnotation: "true",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: utils.Pointer[int32](1),
					Selector: &v1.LabelSelector{
						MatchLabels: map[string]string{"app": "nginx"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: v1.ObjectMeta{
							Labels: map[string]string{"app": "nginx"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, deploy)).To(Succeed())

			rs := &appsv1.ReplicaSet{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-deploy",
					Namespace: "default",
					Labels:    map[string]string{"app": "nginx"},
					OwnerReferences: []v1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "Deployment",
							Name:       deploy.Name,
							UID:        deploy.UID,
							Controller: utils.Pointer(true),
						},
					},
				},
				Spec: appsv1.ReplicaSetSpec{
					Replicas: utils.Pointer[int32](1),
					Selector: &v1.LabelSelector{
						MatchLabels: map[string]string{"app": "nginx"},
					},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: v1.ObjectMeta{
							Labels: map[string]string{"app": "nginx"},
						},
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "nginx",
									Image: "nginx:latest",
								},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, rs)).To(Succeed())
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: rs.Name, Namespace: "default"}, rs)).To(Succeed())

			node := &corev1.Node{
				ObjectMeta: v1.ObjectMeta{
					Name: "test-node",
					Labels: map[string]string{
						"kubernetes.io/hostname": "test-node",
					},
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, node)).To(Succeed())
			pod := &corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Name:      "pod-1",
					Namespace: "default",
					Labels:    map[string]string{"app": "nginx"},
					OwnerReferences: []v1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "ReplicaSet",
							Name:       rs.Name,
							UID:        rs.UID,
							Controller: utils.Pointer(true),
						},
					},
				},
				Spec: corev1.PodSpec{
					NodeName: node.Name,
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			// FETCH BACK (IMPORTANT!)
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			}, pod)).To(Succeed())

			// UPDATE STATUS USING STATUS SUBRESOURCE
			pod.Status = corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:               corev1.PodReady,
						Status:             corev1.ConditionTrue,
						Reason:             "Ready",
						LastTransitionTime: v1.Now(),
					},
				},
			}

			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			// Trigger reconcile through a spec update (status update alone does NOT trigger reconcile)
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			}, pod)).To(Succeed())

			pod.Labels["reconcile-trigger"] = "true"
			Expect(k8sClient.Update(ctx, pod)).To(Succeed())

			Eventually(func() string {
				out := &corev1.Pod{}
				_ = k8sClient.Get(ctx,
					types.NamespacedName{Name: "pod-1", Namespace: "default"},
					out,
				)
				if out.Annotations == nil {
					return ""
				}
				return out.Annotations["controller.kubernetes.io/pod-deletion-cost"]
			}, "10s", "300ms").ShouldNot(BeEmpty())

		})
	})
})
