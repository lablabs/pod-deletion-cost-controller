package controller

import (
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CacheOptions returns the cache options for the manager
func CacheOptions() cache.Options {
	return cache.Options{
		DefaultTransform: cache.TransformStripManagedFields(),
		ByObject: map[client.Object]cache.ByObject{
			&corev1.Pod{}: {Transform: TransformPod},
		},
	}
}

// TransformPod keeps only the Pod fields that the controller reads
func TransformPod(obj interface{}) (interface{}, error) {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return obj, nil
	}
	pod.ManagedFields = nil
	pod.Spec = corev1.PodSpec{NodeName: pod.Spec.NodeName}
	pod.Status = corev1.PodStatus{
		Phase:      pod.Status.Phase,
		Conditions: pod.Status.Conditions,
	}
	return pod, nil
}

// ReplicaSetMetadata returns an empty metadata-only ReplicaSet
func ReplicaSetMetadata() *metav1.PartialObjectMetadata {
	obj := &metav1.PartialObjectMetadata{}
	obj.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ReplicaSet"))
	return obj
}

// ReplicaSetMetadataList returns an empty metadata-only ReplicaSet list
func ReplicaSetMetadataList() *metav1.PartialObjectMetadataList {
	list := &metav1.PartialObjectMetadataList{}
	list.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("ReplicaSetList"))
	return list
}

// DeploymentMetadata returns an empty metadata-only Deployment
func DeploymentMetadata() *metav1.PartialObjectMetadata {
	obj := &metav1.PartialObjectMetadata{}
	obj.SetGroupVersionKind(v1.SchemeGroupVersion.WithKind("Deployment"))
	return obj
}

// NodeMetadata returns an empty metadata-only Node
func NodeMetadata() *metav1.PartialObjectMetadata {
	obj := &metav1.PartialObjectMetadata{}
	obj.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Node"))
	return obj
}
