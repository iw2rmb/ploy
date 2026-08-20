package contracts

import (
	"fmt"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// RepoMaterialization describes repository inputs required for a workflow run.
type RepoMaterialization struct {
	URL           types.RepoURL   `json:"url,omitempty"`
	BaseRef       types.GitRef    `json:"base_ref,omitempty"`
	Commit        types.CommitSHA `json:"commit,omitempty"`
	WorkspaceHint string          `json:"workspace_hint,omitempty"`
}

// Validate ensures repo metadata is well formed when provided.
func (r RepoMaterialization) Validate() error {
	// URL is optional; when set, validate and require either base ref or commit.
	if r.URL != "" {
		if err := r.URL.Validate(); err != nil {
			return fmt.Errorf("url: %w", err)
		}
		if r.BaseRef == "" && r.Commit == "" {
			return fmt.Errorf("base_ref or commit is required when repo url is set")
		}
	}
	// Validate optional refs/commit when provided.
	if r.BaseRef != "" {
		if err := r.BaseRef.Validate(); err != nil {
			return fmt.Errorf("base_ref: %w", err)
		}
	}
	if r.Commit != "" {
		if err := r.Commit.Validate(); err != nil {
			return fmt.Errorf("commit: %w", err)
		}
	}
	return nil
}
