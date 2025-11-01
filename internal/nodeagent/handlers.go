//go:build legacy
// +build legacy

package nodeagent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// StartRunRequest describes a run start request from the server.
type StartRunRequest struct {
	RunID     string            `json:"run_id"`
	RepoURL   string            `json:"repo_url"`
	BaseRef   string            `json:"base_ref"`
	TargetRef string            `json:"target_ref"`
	CommitSHA string            `json:"commit_sha"`
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
	if strings.TrimSpace(req.RunID) == "" {
		return fmt.Errorf("run_id is required")
	}
	if strings.TrimSpace(req.RepoURL) == "" {
		return fmt.Errorf("repo_url is required")
	}
	return nil
}
