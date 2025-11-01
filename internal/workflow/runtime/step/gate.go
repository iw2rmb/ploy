package step

import (
	"context"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// GateExecutor validates build artifacts using the configured profile.
type GateExecutor interface {
	Execute(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error)
}

// gateExecutor implements build gate validation.
type gateExecutor struct {
	now func() time.Time
}

// NewGateExecutor constructs a gate executor.
func NewGateExecutor() GateExecutor {
	return &gateExecutor{
		now: func() time.Time { return time.Now().UTC() },
	}
}

// Execute runs build gate validation when enabled.
func (e *gateExecutor) Execute(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
	_ = ctx
	_ = workspace
	if spec == nil || !spec.Enabled {
		return nil, nil
	}
	// Build gate execution is a placeholder for now.
	// Full implementation will integrate with language-specific checkers
	// and parse build logs for known failure patterns.
	metadata := &contracts.BuildGateStageMetadata{
		StaticChecks: []contracts.BuildGateStaticCheckReport{},
		LogFindings:  []contracts.BuildGateLogFinding{},
	}
	return metadata, nil
}
