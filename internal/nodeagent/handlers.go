package nodeagent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// StartRunRequest describes a run start request from the server.
//
// Options keys (slice E — per-run GitLab/MR wiring):
//   - "gitlab_pat" (string, optional) — PAT override for this run (never log).
//   - "gitlab_domain" (string, optional) — GitLab domain override.
//   - "mr_on_success" (bool) — create MR on success when true.
//   - "mr_on_fail" (bool) — create MR on failure when true.
//
// Other options currently honoured by the node for execution shaping:
//   - "image" (string) — container image to run (optional; default ubuntu:latest).
//   - "command" (string|[]string) — container command override.
//   - "retain_container" (bool) — retain container after run for debugging.
type StartRunRequest struct {
	RunID     types.RunID       `json:"run_id,omitempty"`
	RepoURL   types.RepoURL     `json:"repo_url,omitempty"`
	BaseRef   types.GitRef      `json:"base_ref,omitempty"`
	TargetRef types.GitRef      `json:"target_ref,omitempty"`
	CommitSHA types.CommitSHA   `json:"commit_sha,omitempty"`
	Options   map[string]any    `json:"options"`
	Env       map[string]string `json:"env"`
}

// StartRunResponse is returned when a run is accepted.
type StartRunResponse struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

// StopRunRequest describes a run stop/cancel request.
type StopRunRequest struct {
	RunID  string `json:"run_id"`
	Reason string `json:"reason"`
}

// StopRunResponse is returned when a stop request is processed.
type StopRunResponse struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

func (s *Server) handleRunStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

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
	if err := s.controller.StartRun(ctx, req); err != nil {
		http.Error(w, fmt.Sprintf("start run failed: %v", err), http.StatusInternalServerError)
		return
	}

	resp := StartRunResponse{
		RunID:  req.RunID.String(),
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

	var req StopRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.RunID) == "" {
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
	if strings.TrimSpace(req.RepoURL.String()) == "" {
		return fmt.Errorf("repo_url is required")
	}
	return nil
}
