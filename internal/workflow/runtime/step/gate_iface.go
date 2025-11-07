package step

import (
	"context"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// GateExecutor validates build artifacts using the configured profile.
// Implementations live alongside container runtimes (e.g., gate_docker.go).
// The interface remains here to avoid import cycles.
type GateExecutor interface {
	Execute(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error)
}
