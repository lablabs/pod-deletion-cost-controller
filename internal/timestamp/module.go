package timestamp

import (
	"fmt"
	"slices"

	"github.com/go-logr/logr"
	"github.com/lablabs/pod-deletion-cost-controller/internal/module"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Name is the module name used for registration and enablement.
	Name = "timestamp"
)

// Registrator defines the controller manager interface for module registration.
type Registrator interface {
	AddModule(module module.Handler) error
}

// Register registers the timestamp module if it's in the enabled algorithms list.
func Register(log logr.Logger, r Registrator, client client.Client, algoTypes []string) error {
	if slices.Contains(algoTypes, Name) || len(algoTypes) == 0 {
		h := NewHandler(client)
		err := r.AddModule(h)
		if err != nil {
			return fmt.Errorf("register timestamp module failed: %w", err)
		}
		log.WithValues("module", Name).Info("registered")
		return nil
	}
	log.V(2).WithValues("module", Name).Info("NOT registered")

	return nil
}
