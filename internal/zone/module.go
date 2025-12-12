package zone

import (
	"fmt"

	"github.com/lablabs/pod-deletion-cost-controller/internal/module"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Registrator interface {
	AddModule(module module.Handler) error
}

func Register(r Registrator, client client.Client) error {
	h := NewHandler(client)
	err := r.AddModule(h)
	if err != nil {
		return fmt.Errorf("register zone module failed: %w", err)
	}
	return nil
}
