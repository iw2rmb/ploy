package api

import (
	"encoding/json"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// RunSubmitRequest is the canonical request DTO for POST /v1/runs.
// It is the authoritative shape for single-repo run submission shared between
// the server handler and CLI clients.
//
// Wire shape is stable: JSON field names must not change.
type RunSubmitRequest struct {
	RepoURL   domaintypes.RepoURL `json:"repo_url"`
	Ref       domaintypes.GitRef  `json:"ref"`
	CommitSHA string              `json:"commit_sha,omitempty"`
	SpecID    domaintypes.SpecID  `json:"spec_id,omitempty"`
	Spec      json.RawMessage     `json:"spec"`
	CreatedBy string              `json:"created_by,omitempty"`
}

// CreateSingleRepoRunResponse is returned by POST /v1/runs.
type CreateSingleRepoRunResponse struct {
	WaveID domaintypes.WaveID `json:"wave_id"`
	RunID  domaintypes.RunID  `json:"run_id"`
	MigID  domaintypes.MigID  `json:"mig_id"`
	SpecID domaintypes.SpecID `json:"spec_id"`
}
