package nodeagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// maxRequestBodySize limits request body size to prevent OOM from large payloads.
const maxRequestBodySize = 10 << 20 // 10 MiB

// StartRunRequest describes a run start request from the server.
//
// TypedOptions contains all run configuration options in strongly-typed form.
// This is the canonical source of truth for all option keys understood by the
// nodeagent. Callers must use TypedOptions fields instead of raw map[string]any
// access. The typed options include:
//
//   - BuildGate: enabled flag and image overrides for gate validation.
//   - Healing: retry policy and healing mig spec.
//   - MRWiring: GitLab PAT, domain, and MR creation triggers (mr_on_success, mr_on_fail).
//   - Execution: container image, command, and retention settings.
//   - Artifacts: artifact name and workspace-relative paths to upload.
//   - ServerMetadata: server-injected job ID for upload correlation.
//   - Steps: multi-step migs array for sequential execution.
//
// JobType field:
//   - Identifies the job type: "pre_gate", "mig", "post_gate".
//   - Used by orchestrator to dispatch to appropriate execution handler.
type StartRunRequest struct {
	RunID   types.RunID     `json:"run_id,omitempty"`
	JobID   types.JobID     `json:"job_id,omitempty"`   // Job ID for artifact/diff uploads
	RepoID  types.MigRepoID `json:"repo_id,omitempty"`  // Repo ID (NanoID) for repo-scoped artifacts (diffs/logs)
	RepoURL types.RepoURL   `json:"repo_url,omitempty"` // Repository URL for this run
	// RepoGateProfileMissing is set by the control plane claim response when
	// mig_repos.gate_profile is currently empty for the claimed repo.
	RepoGateProfileMissing bool `json:"repo_gate_profile_missing,omitempty"`
	// Name is an optional human-friendly run name provided by the control plane.
	// When set (e.g., for batch runs), it can be used for branch naming in MR flows.
	Name      string          `json:"name,omitempty"`
	BaseRef   types.GitRef    `json:"base_ref,omitempty"`
	TargetRef types.GitRef    `json:"target_ref,omitempty"`
	CommitSHA types.CommitSHA `json:"commit_sha,omitempty"`
	RepoSHAIn types.CommitSHA `json:"repo_sha_in,omitempty"`
	JobType   types.JobType   `json:"job_type,omitempty"`  // Job type: pre_gate, mig, post_gate
	JobImage  string          `json:"job_image,omitempty"` // Container image for this job
	NextID    *types.JobID    `json:"next_id,omitempty"`   // Linked successor in run chain
	JobName   string          `json:"job_name,omitempty"`  // Deprecated: kept for wire compatibility during context rollout.
	// MigContext carries concrete mig step routing.
	MigContext *contracts.MigClaimContext `json:"mig_context,omitempty"`
	// GateContext carries concrete gate cycle routing.
	GateContext *contracts.GateClaimContext `json:"gate_context,omitempty"`
	// DetectedStack carries the canonical gate-detected stack tuple for this job.
	DetectedStack *contracts.StackExpectation `json:"detected_stack,omitempty"`
	// RecoveryContext carries server-resolved recovery inputs.
	RecoveryContext *contracts.RecoveryClaimContext `json:"recovery_context,omitempty"`
	// TypedOptions contains strongly-typed run configuration. This is the canonical
	// source of truth for all option keys understood by the nodeagent. Execution,
	// healing, manifest building, and artifact upload paths all consume TypedOptions
	// directly rather than parsing raw maps.
	TypedOptions RunOptions        `json:"-"`   // Not serialized; populated by claimer_loop from parsed spec
	Env          map[string]string `json:"env"` // Environment variables merged from spec
}

// StartActionRequest describes an action execution request from the server claim API.
// Actions are terminal follow-up tasks and are not part of the jobs chain.
type StartActionRequest struct {
	ActionID     types.JobID       `json:"action_id,omitempty"`
	ActionType   string            `json:"action_type,omitempty"`
	RunID        types.RunID       `json:"run_id,omitempty"`
	RepoID       types.MigRepoID   `json:"repo_id,omitempty"`
	RepoURL      types.RepoURL     `json:"repo_url,omitempty"`
	BaseRef      types.GitRef      `json:"base_ref,omitempty"`
	TargetRef    types.GitRef      `json:"target_ref,omitempty"`
	TypedOptions RunOptions        `json:"-"`
	Env          map[string]string `json:"env"`
}

// StartRunResponse is returned when a run is accepted.
type StartRunResponse struct {
	RunID  types.RunID `json:"run_id"`
	Status string      `json:"status"`
}

// StopRunRequest describes a run stop/cancel request.
type StopRunRequest struct {
	RunID  types.RunID `json:"run_id"`
	Reason string      `json:"reason"`
}

// StopRunResponse is returned when a stop request is processed.
type StopRunResponse struct {
	RunID  types.RunID `json:"run_id"`
	Status string      `json:"status"`
}

func (s *Server) handleRunStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit request body size to prevent OOM attacks.
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req StartRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if err := validateStartRunRequest(req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if err := s.controller.AcquireSlot(ctx); err != nil {
		// If the request is canceled or times out while waiting, treat it as a
		// client-side timeout.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			http.Error(w, err.Error(), http.StatusRequestTimeout)
			return
		}
		http.Error(w, fmt.Sprintf("acquire concurrency slot failed: %v", err), http.StatusInternalServerError)
		return
	}

	slotHeld := true
	defer func() {
		if slotHeld {
			s.controller.ReleaseSlot()
		}
	}()

	if err := s.controller.StartRun(ctx, req); err != nil {
		http.Error(w, fmt.Sprintf("start run failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Transfer slot ownership to the controller. The slot is released when the
	// job completes.
	slotHeld = false

	resp := StartRunResponse{
		RunID:  req.RunID,
		Status: "accepted",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleRunStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit request body size to prevent OOM attacks.
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req StopRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.RunID.IsZero() {
		http.Error(w, "run_id is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	if err := s.controller.StopRun(ctx, req); err != nil {
		http.Error(w, fmt.Sprintf("stop run failed: %v", err), http.StatusInternalServerError)
		return
	}

	resp := StopRunResponse{
		RunID:  req.RunID,
		Status: "stopped",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func validateStartRunRequest(req StartRunRequest) error {
	if req.RunID.IsZero() {
		return fmt.Errorf("run_id is required")
	}
	if req.JobID.IsZero() {
		return fmt.Errorf("job_id is required")
	}
	if req.JobType.IsZero() {
		return fmt.Errorf("job_type is required")
	}
	if err := req.JobType.Validate(); err != nil {
		return fmt.Errorf("job_type invalid: %w", err)
	}
	if strings.TrimSpace(req.RepoURL.String()) == "" {
		return fmt.Errorf("repo_url is required")
	}
	return nil
}
