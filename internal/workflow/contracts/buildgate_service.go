package contracts

import (
	"fmt"
	"strings"
)

// BuildGateJobStatus represents the current state of a build gate validation job.
type BuildGateJobStatus string

const (
	BuildGateJobStatusPending   BuildGateJobStatus = "pending"
	BuildGateJobStatusClaimed   BuildGateJobStatus = "claimed"
	BuildGateJobStatusRunning   BuildGateJobStatus = "running"
	BuildGateJobStatusCompleted BuildGateJobStatus = "completed"
	BuildGateJobStatusFailed    BuildGateJobStatus = "failed"
)

// BuildGateValidateRequest is the request payload for POST /v1/buildgate/validate.
// It supports two modes:
// 1. Ref-based: provide RepoURL and Ref (service clones repo)
// 2. Content-based: provide ContentArchive (tarball/zip bytes)
type BuildGateValidateRequest struct {
	// Ref-based validation fields
	RepoURL string `json:"repo_url,omitempty"`
	Ref     string `json:"ref,omitempty"`

	// Content-based validation: base64-encoded tarball or zip archive
	ContentArchive []byte `json:"content_archive,omitempty"`

	// Common fields
	Profile string `json:"profile,omitempty"` // auto, java, java-maven, java-gradle
	Timeout string `json:"timeout,omitempty"` // duration string (e.g., "5m")

	// Resource limits
	LimitMemoryBytes *int64 `json:"limit_memory_bytes,omitempty"`
	LimitCPUMillis   *int64 `json:"limit_cpu_millis,omitempty"`
	LimitDiskSpace   *int64 `json:"limit_disk_space,omitempty"`
}

// Validate ensures the request has either ref-based or content-based fields.
func (r BuildGateValidateRequest) Validate() error {
	hasRef := strings.TrimSpace(r.RepoURL) != "" || strings.TrimSpace(r.Ref) != ""
	hasContent := len(r.ContentArchive) > 0

	if !hasRef && !hasContent {
		return fmt.Errorf("either repo_url+ref or content_archive must be provided")
	}
	if hasRef && hasContent {
		return fmt.Errorf("cannot provide both repo_url+ref and content_archive")
	}

	// If ref-based, both repo_url and ref are required
	if hasRef {
		if strings.TrimSpace(r.RepoURL) == "" {
			return fmt.Errorf("repo_url is required for ref-based validation")
		}
		if strings.TrimSpace(r.Ref) == "" {
			return fmt.Errorf("ref is required for ref-based validation")
		}
	}

	// Validate resource limits
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
