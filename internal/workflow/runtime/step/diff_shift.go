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

// ShiftClient invokes the SHIFT build gate.
type ShiftClient interface {
	Validate(ctx context.Context, req ShiftRequest) (ShiftResult, error)
}

// ShiftRequest wraps manifest + workspace context.
type ShiftRequest struct {
	Manifest    contracts.StepManifest
	Workspace   Workspace
	LogArtifact *PublishedArtifact
}

// ShiftResult contains SHIFT execution details.
type ShiftResult struct {
	Passed   bool
	Message  string
	Report   []byte
	Duration time.Duration
}
