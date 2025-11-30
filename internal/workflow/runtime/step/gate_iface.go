package step

import (
	"context"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// GateExecutor validates build artifacts using the configured profile.
// Implementations live alongside container runtimes (e.g., gate_docker.go).
// The interface remains here to avoid import cycles.
type GateExecutor interface {
	Execute(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error)
}

// BuildGateHTTPClient defines the HTTP client interface for the Build Gate API.
// Used by HTTPGateExecutor to submit validation requests and poll job status.
// The client handles POST /v1/buildgate/validate and GET /v1/buildgate/jobs/{id}.
type BuildGateHTTPClient interface {
	// Validate submits a build validation request to the Build Gate API.
	// Returns a response containing either an immediate result (sync completion)
	// or a job ID for polling (async execution).
	Validate(ctx context.Context, req contracts.BuildGateValidateRequest) (*contracts.BuildGateValidateResponse, error)

	// GetJob retrieves the current status of a build gate job by ID.
	// Used for polling when Validate returns a pending job.
	GetJob(ctx context.Context, jobID types.JobID) (*contracts.BuildGateJobStatusResponse, error)
}
