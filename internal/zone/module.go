package zone

import (
	"fmt"

	"github.com/go-logr/logr"
	"github.com/lablabs/pod-deletion-cost-controller/internal/module"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	//Name of module
	Name = "zone"
)

// Registrator define controller manager
type Registrator interface {
	AddModule(module module.Handler) error
}

// Register register zone module
func Register(log logr.Logger, r Registrator, client client.Client, algoTypes []string) error {
	if slices.Contains(algoTypes, Name) || len(algoTypes) == 0 {
		h := NewHandler(client)
		err := r.AddModule(h)
		if err != nil {
			return fmt.Errorf("register zone module failed: %w", err)
		}
		log.WithValues("module", Name).Info("registered")
		return nil
	}
	log.V(2).WithValues("module", Name).Info("NOT registered")

	return nil
}
