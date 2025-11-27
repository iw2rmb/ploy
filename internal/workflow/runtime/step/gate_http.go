// gate_http.go implements the HTTP-based GateExecutor for remote build validation.
//
// This executor delegates build validation to the Build Gate API via HTTP, enabling
// gate jobs to run on any eligible Build Gate worker node rather than on the local
// node agent. This decouples gate execution from the Mods execution node, supporting
// multi-VPS architectures where dedicated Build Gate workers handle validation.
//
// ## Usage
//
// HTTPGateExecutor requires a BuildGateHTTPClient (see gate_http_client.go) for
// communication with the Build Gate API. Create one via NewHTTPGateExecutor:
//
//	client, _ := NewBuildGateHTTPClient(cfg)
//	executor := NewHTTPGateExecutor(client)
//	result, err := executor.Execute(ctx, spec, workspace)
//
// ## Sync vs Async Execution
//
// This implementation supports SYNC ONLY mode:
//   - If Validate returns an immediate result (Status=Completed), it is returned directly.
//   - If Validate returns Status=Pending, an error is returned indicating async jobs
//     are not yet supported. Phase B3 will add polling support.
//
// ## Relationship to DockerGateExecutor
//
// HTTPGateExecutor and dockerGateExecutor both implement GateExecutor, allowing
// configuration-driven selection (Phase B4). The HTTP executor sends validation
// requests to remote workers, while the Docker executor runs builds locally.
// Both produce equivalent BuildGateStageMetadata for consistent downstream handling.
package step

import (
	"context"
	"errors"
	"fmt"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// httpGateExecutor implements GateExecutor by delegating to the Build Gate HTTP API.
// This enables gate validation to run on remote Build Gate workers rather than locally.
type httpGateExecutor struct {
	// client handles HTTP communication with the Build Gate API.
	client BuildGateHTTPClient
}

// NewHTTPGateExecutor constructs a GateExecutor that uses the Build Gate HTTP API
// for remote validation. The provided client must implement BuildGateHTTPClient
// (typically created via NewBuildGateHTTPClient).
//
// Returns nil if client is nil, allowing callers to gracefully degrade when
// HTTP-based gate execution is not configured.
func NewHTTPGateExecutor(client BuildGateHTTPClient) GateExecutor {
	if client == nil {
		return nil
	}
	return &httpGateExecutor{client: client}
}

// Execute submits a build validation request to the Build Gate HTTP API.
//
// Behavior mirrors dockerGateExecutor for consistency:
//   - Returns (nil, nil) when spec is nil or spec.Enabled is false.
//   - Returns (nil, ctx.Err()) if the context is already cancelled.
//
// Request construction:
//   - Profile and Timeout are taken from spec if provided.
//   - RepoURL and Ref are temporary placeholders; Phase C will wire repo+diff
//     metadata from the step manifest.
//
// Response handling:
//   - If the API returns an immediate result (Status != Pending), Result is returned.
//   - If the API returns Status=Pending (async job), an error is returned. Phase B3
//     will implement polling via client.GetJob().
//
// The workspace parameter is currently unused for HTTP mode since remote workers
// receive repo+diff payloads rather than direct workspace access.
func (e *httpGateExecutor) Execute(ctx context.Context, spec *contracts.StepGateSpec, workspace string) (*contracts.BuildGateStageMetadata, error) {
	// Honor context cancellation early (consistent with dockerGateExecutor).
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Early return for disabled or nil spec (mirrors dockerGateExecutor behavior).
	if spec == nil || !spec.Enabled {
		return nil, nil
	}

	// Build a minimal request. Phase C (C1) will wire repo metadata (RepoURL, Ref)
	// from the step manifest. For now, these are temporary placeholders to satisfy
	// the BuildGateValidateRequest.Validate() requirements.
	//
	// NOTE: Callers must ensure RepoURL and Ref are populated before using this
	// executor in production. This skeleton uses placeholder values that will fail
	// validation in the HTTP client unless the spec carries real repo metadata.
	req := contracts.BuildGateValidateRequest{
		// Temporary: placeholder values. Phase C will populate from step manifest.
		// The HTTP client will return a validation error if these remain empty,
		// which is the expected behavior until repo+diff wiring is complete.
		RepoURL: "", // TODO(B2): Wire from step manifest in Phase C.
		Ref:     "", // TODO(B2): Wire from step manifest in Phase C.

		// Profile from spec if provided; empty string triggers auto-detection on server.
		Profile: spec.Profile,
		// Timeout: StepGateSpec doesn't have timeout; use server default. Phase C may
		// add timeout from manifest or env (PLOY_BUILDGATE_TIMEOUT).
	}

	// Submit validation request to the Build Gate API.
	resp, err := e.client.Validate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("validate via http: %w", err)
	}

	// Handle response based on job status.
	switch resp.Status {
	case contracts.BuildGateJobStatusCompleted:
		// Sync completion: result is available immediately.
		if resp.Result != nil {
			return resp.Result, nil
		}
		// Completed status but no result is unexpected; treat as empty metadata.
		return &contracts.BuildGateStageMetadata{}, nil

	case contracts.BuildGateJobStatusFailed:
		// Job failed on the server. Return empty metadata with error.
		// The server may have stored partial results; for now, return an error.
		return nil, fmt.Errorf("build gate job failed: job_id=%s", resp.JobID)

	case contracts.BuildGateJobStatusPending, contracts.BuildGateJobStatusClaimed, contracts.BuildGateJobStatusRunning:
		// Async job: polling not yet implemented.
		// Phase B3 will add polling via client.GetJob() until completion.
		return nil, errors.New("async jobs not supported yet")

	default:
		// Unknown status: defensive error handling.
		return nil, fmt.Errorf("unexpected job status: %s", resp.Status)
	}
}
