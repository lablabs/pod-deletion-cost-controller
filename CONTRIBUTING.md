# Contributing to Pod Deletion Cost Controller

Thank you for your interest in contributing to the Pod Deletion Cost Controller! This document provides guidelines for contributing, with a focus on adding new algorithms.

## Table of Contents

- [Architecture](#architecture)
- [Development Setup](#development-setup)
- [Build & Test](#build--test)
- [Adding New Algorithms](#adding-new-algorithms)
- [Code Style](#code-style)
- [Testing Requirements](#testing-requirements)
- [Pull Request Process](#pull-request-process)

## Architecture

The controller uses a plugin-based architecture for extensibility:

```
┌─────────────────────────────────────────────────────┐
│                  PodReconciler                      │
│  (Watches Pods, ReplicaSets, Deployments, Nodes)   │
└─────────────────────┬───────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────┐
│                 ModuleManager                       │
│      (Routes to appropriate algorithm handler)      │
└─────────────────────┬───────────────────────────────┘
                      │
        ┌─────────────┼─────────────┐
        ▼             ▼             ▼
   ┌─────────┐  ┌─────────┐  ┌─────────┐
   │  Zone   │  │ Future  │  │ Future  │
   │ Handler │  │  Algo   │  │  Algo   │
   └─────────┘  └─────────┘  └─────────┘
```

### Key Components

- **PodReconciler**: Main controller that watches Kubernetes resources and triggers reconciliation
- **ModuleManager**: Routes pods to the appropriate algorithm handler based on Deployment annotations
- **Handler**: Algorithm implementations (currently `zone`)
- **Expectations Cache**: Thread-safe cache for handling async reconciliation

### Project Structure

```
├── cmd/
│   └── main.go                    # Entry point, module registration
├── internal/
│   ├── controller/                # Core reconciler logic
│   │   ├── pod_controller.go      # PodReconciler
│   │   ├── modules.go             # ModuleManager
│   │   ├── annotation.go          # Annotation helpers
│   │   ├── lookup.go              # K8s object traversal
│   │   └── predicate.go           # Event predicates
│   ├── zone/                      # Zone algorithm implementation
│   │   ├── handler.go             # Zone distribution handler
│   │   ├── controller_utils.go    # DeletionCostPool
│   │   └── module.go              # Module registration
│   ├── module/                    # Module interface definitions
│   │   └── handler.go             # Handler interface
│   └── expectations/              # Caching layer
│       └── cache.go               # Generic sync cache
├── charts/                        # Helm chart
└── test/e2e/                      # End-to-end tests
```

## Development Setup

### Prerequisites

- [Go](https://golang.org/) 1.24+
- [mise](https://mise.jdx.dev/) - for tool management
- [Kind](https://kind.sigs.k8s.io/) - for E2E tests
- [pre-commit](https://pre-commit.com/) - for git hooks

### Getting Started

```bash
# Clone the repository
git clone https://github.com/lablabs/pod-deletion-cost-controller.git
cd pod-deletion-cost-controller

# Install pre-commit hooks
pre-commit install

# Install dependencies
go mod download

# Run tests to verify setup
make test
```

## Build & Test

```bash
# Build
make build

# Run unit tests
make test

# Run E2E tests (requires Kind)
make test-e2e

# Lint
make lint
```

### Docker

```bash
# Build image
make docker-build IMG=myregistry/pod-deletion-cost-controller:tag

# Push image
make docker-push IMG=myregistry/pod-deletion-cost-controller:tag
```

## Adding New Algorithms

The controller uses a plugin-based architecture that makes it easy to add new algorithms. Each algorithm is a "module" that implements the `Handler` interface.

### Step 1: Create a New Package

Create a new directory under `internal/` for your algorithm:

```
internal/
├── controller/
├── expectations/
├── module/
├── zone/           # Existing zone algorithm
└── myalgo/         # Your new algorithm
    ├── handler.go
    ├── handler_test.go
    ├── module.go
    └── annotation.go  # Optional: if you need custom annotations
```

### Step 2: Implement the Handler Interface

Your algorithm must implement the `Handler` interface defined in `internal/module/handler.go`:

```go
package module

import (
    "context"

    "github.com/go-logr/logr"
    appv1 "k8s.io/api/apps/v1"
    corev1 "k8s.io/api/core/v1"
)

// Handler represent main module interface
type Handler interface {
    // AcceptType returns the algorithm type(s) this handler accepts
    // The values correspond to the pod-deletion-cost.lablabs.io/type annotation
    AcceptType() []string

    // Handle processes a pod and applies the appropriate deletion cost
    Handle(ctx context.Context, log logr.Logger, pod *corev1.Pod, dep *appv1.Deployment) error
}
```

### Step 3: Create Your Handler

Create `internal/myalgo/handler.go`:

```go
package myalgo

import (
    "context"
    "fmt"

    "github.com/go-logr/logr"
    "github.com/lablabs/pod-deletion-cost-controller/internal/controller"
    "github.com/lablabs/pod-deletion-cost-controller/internal/expectations"
    v1 "k8s.io/api/apps/v1"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/types"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
    // TypeAnnotation is the value for pod-deletion-cost.lablabs.io/type
    TypeAnnotation = "myalgo"
)

// NewHandler creates a new Handler
func NewHandler(client client.Client) *Handler {
    return &Handler{
        client: client,
        cache:  expectations.NewCache[types.UID, int](),
    }
}

// Handler handles reconcile loop for Pod/Deployment
type Handler struct {
    client client.Client
    cache  *expectations.Cache[types.UID, int]
}

// AcceptType returns accepted algorithm types
func (h *Handler) AcceptType() []string {
    return []string{TypeAnnotation}
}

// Handle implements your algorithm logic
func (h *Handler) Handle(ctx context.Context, log logr.Logger, pod *corev1.Pod, dep *v1.Deployment) error {
    // Skip if pod already has deletion cost annotation
    if controller.HasPodDeletionCost(pod) {
        h.cache.Delete(pod.UID)
        log.V(3).Info("clean cache, pod was sync")
        return nil
    }

    // Skip pods being deleted
    if controller.IsDeleting(pod) {
        return nil
    }

    // Your algorithm logic here
    // Calculate the deletion cost based on your criteria
    cost := calculateCost(pod, dep)

    // Cache the value for async reconciliation
    h.cache.Set(pod.UID, cost)

    // Apply the annotation
    patch := client.MergeFrom(pod.DeepCopy())
    controller.ApplyPodDeletionCost(pod, cost)
    err := h.client.Patch(ctx, pod, patch)
    if err != nil {
        return err
    }

    log.WithValues(controller.PodDeletionCostAnnotation, cost).Info("updated")
    return nil
}

func calculateCost(pod *corev1.Pod, dep *v1.Deployment) int {
    // Implement your cost calculation logic
    // Return a value between -2147483648 and 2147483647
    // Higher values = lower deletion priority (deleted last)
    // Lower values = higher deletion priority (deleted first)
    return 0
}
```

### Step 4: Create the Module Registration

Create `internal/myalgo/module.go`:

```go
package myalgo

import (
    "fmt"

    "github.com/go-logr/logr"
    "github.com/lablabs/pod-deletion-cost-controller/internal/module"
    "k8s.io/utils/strings/slices"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
    // Name of module
    Name = "myalgo"
)

// Registrator defines controller manager interface
type Registrator interface {
    AddModule(module module.Handler) error
}

// Register registers the module with the controller
func Register(log logr.Logger, r Registrator, client client.Client, algoTypes []string) error {
    // Register if this algorithm is in the enabled list, or if no list specified
    if slices.Contains(algoTypes, Name) || len(algoTypes) == 0 {
        h := NewHandler(client)
        err := r.AddModule(h)
        if err != nil {
            return fmt.Errorf("register %s module failed: %w", Name, err)
        }
        log.WithValues("module", Name).Info("registered")
        return nil
    }
    log.V(2).WithValues("module", Name).Info("NOT registered")
    return nil
}
```

### Step 5: Register in main.go

Add your module registration in `cmd/main.go`:

```go
import (
    // ... existing imports
    "github.com/lablabs/pod-deletion-cost-controller/internal/myalgo"
)

func main() {
    // ... existing code ...

    // Configuration part for algorithms
    moduleMng := controller.NewModuleManager()

    // Register existing zone handler
    err = zone.Register(logger, moduleMng, mgr.GetClient(), algoType)
    if err != nil {
        logger.Error(err, "unable to register zone")
        os.Exit(1)
    }

    // Register your new algorithm handler
    err = myalgo.Register(logger, moduleMng, mgr.GetClient(), algoType)
    if err != nil {
        logger.Error(err, "unable to register myalgo")
        os.Exit(1)
    }

    // ... rest of existing code ...
}
```

### Step 6: Update Helm Chart (Optional)

If your algorithm should be enabled by default or needs configuration, update `charts/pod-deletion-cost-controller/values.yaml`:

```yaml
algorithms:
  - "zone"
  - "myalgo"  # Add your algorithm
```

### Available Helper Functions

The `internal/controller` package provides useful helper functions:

```go
// Annotation helpers
controller.HasPodDeletionCost(pod *corev1.Pod) bool
controller.GetPodDeletionCost(pod *corev1.Pod) (int, bool)
controller.ApplyPodDeletionCost(pod *corev1.Pod, cost int)
controller.IsDeleting(pod *corev1.Pod) bool

// Deployment helpers
controller.IsEnabled(dep *v1.Deployment) bool
controller.GetType(dep *v1.Deployment) string
```

### Using the Expectations Cache

The `expectations.Cache` helps handle async reconciliation by tracking pending assignments:

```go
cache := expectations.NewCache[types.UID, int]()

// Store a pending value
cache.Set(pod.UID, cost)

// Check if a value exists
if value, exists := cache.Get(pod.UID); exists {
    // Use cached value
}

// Remove when confirmed
cache.Delete(pod.UID)

// Check existence
if cache.Has(pod.UID) {
    // ...
}
```

## Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting
- Run `make lint` before committing
- Add comments for exported functions and types
- Keep functions focused and small

## Testing Requirements

### Unit Tests

Every new algorithm must include unit tests. Create `internal/myalgo/handler_test.go`:

```go
package myalgo

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestMyAlgo(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "MyAlgo Suite")
}

var _ = Describe("MyAlgo Handler", func() {
    Context("when calculating deletion cost", func() {
        It("should assign correct costs", func() {
            // Your test logic
        })
    })
})
```

### Running Tests

```bash
# Run unit tests
make test

# Run E2E tests (requires Kind)
make test-e2e

# Run specific test
go test ./internal/myalgo/... -v
```

### Test Coverage

Aim for at least 80% code coverage for new algorithms.

## Pull Request Process

1. **Fork the repository** and create a feature branch
2. **Implement your changes** following the guidelines above
3. **Write tests** for all new functionality
4. **Run the full test suite**: `make test && make test-e2e`
5. **Run linting**: `make lint`
6. **Update documentation** if needed
7. **Submit a pull request** with a clear description

### PR Checklist

- [ ] Code follows the project's style guidelines
- [ ] Unit tests added/updated
- [ ] E2E tests added/updated (if applicable)
- [ ] Documentation updated
- [ ] All tests pass
- [ ] Linting passes
- [ ] Commit messages are clear and descriptive

## Algorithm Ideas

Looking for inspiration? Here are some algorithm ideas that could be valuable:

- **Age-based**: Prioritize deletion of newer/older pods
- **Resource-based**: Consider pod resource usage for deletion priority
- **Node-type-based**: Spread deletions across different node types (spot vs on-demand)
- ...
## Questions?

If you have questions about contributing or implementing a new algorithm, please open an issue on GitHub.
