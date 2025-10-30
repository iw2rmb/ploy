package step

import (
	"context"
	"time"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// DiffGenerator captures diffs after execution.
type DiffGenerator interface {
	Capture(ctx context.Context, req DiffRequest) (DiffResult, error)
}

// DiffRequest contains diff capture metadata.
type DiffRequest struct {
	Manifest  contracts.StepManifest
	Workspace Workspace
}

// DiffResult summarises the captured diff artifact.
type DiffResult struct {
	Path string
}

// GateClient invokes the Build Gate validation.
type GateClient interface {
    Validate(ctx context.Context, req GateRequest) (GateResult, error)
}

// GateRequest wraps manifest + workspace context.
type GateRequest struct {
    Manifest    contracts.StepManifest
    Workspace   Workspace
    LogArtifact *PublishedArtifact
}

// GateResult contains Build Gate execution details.
type GateResult struct {
    Passed   bool
    Message  string
    Report   []byte
    Duration time.Duration
}
