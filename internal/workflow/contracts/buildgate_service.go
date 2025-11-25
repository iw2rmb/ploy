package contracts

import (
	"fmt"
	"strings"

	"github.com/iw2rmb/ploy/internal/store"
)

// BuildGateJobStatus is an alias for the canonical store.BuildgateJobStatus type.
// This eliminates duplication and ensures consistency across workflow contracts and database layer.
type BuildGateJobStatus = store.BuildgateJobStatus

// Re-export constants for backward compatibility at API boundaries.
const (
	BuildGateJobStatusPending   = store.BuildgateJobStatusPending
	BuildGateJobStatusClaimed   = store.BuildgateJobStatusClaimed
	BuildGateJobStatusRunning   = store.BuildgateJobStatusRunning
	BuildGateJobStatusCompleted = store.BuildgateJobStatusCompleted
	BuildGateJobStatusFailed    = store.BuildgateJobStatusFailed
)

// BuildGateValidateRequest is the request payload for POST /v1/buildgate/validate.
// Requires repo_url and ref for Git-based build validation. The content_archive
// mode has been removed to simplify the Build Gate contract and avoid large
// workspace payloads over HTTP.
//
// Optionally, callers may supply a DiffPatch (gzipped unified diff, base64-encoded)
// to apply on top of the cloned repo_url+ref baseline. This enables healing flows
// to replay changes without shipping full workspace archives.
type BuildGateValidateRequest struct {
	// RepoURL is the Git repository URL to clone. Required.
	RepoURL string `json:"repo_url"`
	// Ref is the Git ref (branch, tag, or commit SHA) to validate. Required.
	Ref string `json:"ref"`

	// DiffPatch is an optional gzipped unified diff (base64-encoded) to apply
	// on top of the cloned repo_url+ref baseline. Used by healing mods to verify
	// changes without transmitting full workspace archives.
	//
	// Semantics: if non-empty, the executor clones repo_url at ref, then applies
	// the decoded/decompressed patch via "git apply" before running the build.
	DiffPatch []byte `json:"diff_patch,omitempty"`

	// Profile specifies the build profile (e.g., auto, java, java-maven, java-gradle).
	Profile string `json:"profile,omitempty"`
	// Timeout is the maximum duration for the build validation (e.g., "5m").
	Timeout string `json:"timeout,omitempty"`

	// Resource limits for the build validation job.
	LimitMemoryBytes *int64 `json:"limit_memory_bytes,omitempty"`
	LimitCPUMillis   *int64 `json:"limit_cpu_millis,omitempty"`
	LimitDiskSpace   *int64 `json:"limit_disk_space,omitempty"`
}

// Validate ensures the request has required repo_url and ref fields.
// DiffPatch is optional but only valid when both repo_url and ref are present.
func (r BuildGateValidateRequest) Validate() error {
	// Both repo_url and ref are required for Git-based build validation.
	repoURLEmpty := strings.TrimSpace(r.RepoURL) == ""
	refEmpty := strings.TrimSpace(r.Ref) == ""

	// DiffPatch requires a valid baseline (repo_url + ref).
	if len(r.DiffPatch) > 0 && (repoURLEmpty || refEmpty) {
		return fmt.Errorf("diff_patch requires both repo_url and ref")
	}

	if repoURLEmpty {
		return fmt.Errorf("repo_url is required")
	}
	if refEmpty {
		return fmt.Errorf("ref is required")
	}

	// Validate resource limits are non-negative when provided.
	if r.LimitMemoryBytes != nil && *r.LimitMemoryBytes < 0 {
		return fmt.Errorf("limit_memory_bytes cannot be negative")
	}
	if r.LimitCPUMillis != nil && *r.LimitCPUMillis < 0 {
		return fmt.Errorf("limit_cpu_millis cannot be negative")
	}
	if r.LimitDiskSpace != nil && *r.LimitDiskSpace < 0 {
		return fmt.Errorf("limit_disk_space cannot be negative")
	}

	return nil
}

// BuildGateValidateResponse is the response for POST /v1/buildgate/validate.
// If the validation completes within the sync timeout, Result will be populated.
// Otherwise, JobID is returned and the client should poll GET /v1/buildgate/jobs/{id}.
type BuildGateValidateResponse struct {
	JobID  string                  `json:"job_id"`
	Status BuildGateJobStatus      `json:"status"`
	Result *BuildGateStageMetadata `json:"result,omitempty"` // populated if sync completion
}

// BuildGateJobStatusResponse is the response for GET /v1/buildgate/jobs/{id}.
type BuildGateJobStatusResponse struct {
	JobID      string                  `json:"job_id"`
	Status     BuildGateJobStatus      `json:"status"`
	Result     *BuildGateStageMetadata `json:"result,omitempty"`
	Error      string                  `json:"error,omitempty"`
	CreatedAt  string                  `json:"created_at"`
	StartedAt  *string                 `json:"started_at,omitempty"`
	FinishedAt *string                 `json:"finished_at,omitempty"`
}
