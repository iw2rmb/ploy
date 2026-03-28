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
	BaseRef   domaintypes.GitRef  `json:"base_ref"`
	TargetRef domaintypes.GitRef  `json:"target_ref"`
	Spec      json.RawMessage     `json:"spec"`
	CreatedBy string              `json:"created_by,omitempty"`
}
