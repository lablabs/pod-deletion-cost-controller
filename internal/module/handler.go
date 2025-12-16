package module

import (
	"context"

	"github.com/go-logr/logr"
	v2 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
)

// Handler represent main module interface
type Handler interface {
	AcceptType() []string
	Handle(ctx context.Context, log logr.Logger, pod *v1.Pod, dep *v2.Deployment) error
}
