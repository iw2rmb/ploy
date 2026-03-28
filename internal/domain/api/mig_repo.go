package api

import (
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// MigRepoSummary is the canonical DTO for a single repo entry within a mig's repo set.
// It is the authoritative response shape for:
//   - GET /v1/migs/{id}/repos — individual items in the repo list
//   - POST /v1/migs/{id}/repos — the added repo in the 201 response
//
// Wire shape is stable: JSON field names must not change.
type MigRepoSummary struct {
	ID        domaintypes.MigRepoID `json:"id"`
	MigID     domaintypes.MigID     `json:"mig_id"`
	RepoURL   string                `json:"repo_url"`
	BaseRef   string                `json:"base_ref"`
	TargetRef string                `json:"target_ref"`
	CreatedAt time.Time             `json:"created_at"`
}

// MigRepoListResponse is the canonical response envelope for GET /v1/migs/{id}/repos.
type MigRepoListResponse struct {
	Repos []MigRepoSummary `json:"repos"`
}
