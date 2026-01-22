// gate_iface.go defines the GateExecutor interface for build validation.
//
// Gate execution always uses the Docker-based executor (gate_docker.go) which
// runs validation containers locally. The interface enables testability and
// allows the Runner to remain agnostic to the underlying implementation.
package step

import (
	"context"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// GateExecutor validates build artifacts.
// The only implementation is dockerGateExecutor (gate_docker.go) which runs
// validation containers locally via the container runtime.
type GateExecutor interface {
	Execute(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error)
}
