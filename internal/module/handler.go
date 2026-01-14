package module

import (
	"context"

	"github.com/go-logr/logr"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// Handler represent main module interface
type Handler interface {
	AcceptType() []string
	Handle(ctx context.Context, log logr.Logger, pod *corev1.Pod, dep *appv1.Deployment) error
}
