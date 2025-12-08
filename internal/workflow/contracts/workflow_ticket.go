package contracts

import (
	"fmt"

	types "github.com/iw2rmb/ploy/internal/domain/types"
)

// WorkflowRun is the envelope used when submitting or claiming a workflow
// run. It carries the schema version, the opaque run identifier, the
// manifest reference (name/version), and optional repository materialization
// details for nodes to hydrate workspaces.
type WorkflowRun struct {
	SchemaVersion string              `json:"schema_version"`
	RunID         types.RunID         `json:"run_id"`
	Manifest      ManifestReference   `json:"manifest"`
	Repo          RepoMaterialization `json:"repo,omitempty"`
}

// Validate checks that required fields are present and that embedded
// structures are valid. It requires a non‑empty schema version and run ID,
// a valid `Manifest`, and (when provided) a valid `Repo`.
func (r WorkflowRun) Validate() error {
	if r.SchemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if r.RunID.IsZero() {
		return fmt.Errorf("run_id is required")
	}
	if err := r.Manifest.Validate(); err != nil {
		return fmt.Errorf("manifest invalid: %w", err)
	}
	if err := r.Repo.Validate(); err != nil {
		return fmt.Errorf("repo invalid: %w", err)
	}
	return nil
}

// RepoMaterialization describes repository inputs required for a workflow run.
type RepoMaterialization struct {
	URL           types.RepoURL   `json:"url,omitempty"`
	BaseRef       types.GitRef    `json:"base_ref,omitempty"`
	TargetRef     types.GitRef    `json:"target_ref,omitempty"`
	Commit        types.CommitSHA `json:"commit,omitempty"`
	WorkspaceHint string          `json:"workspace_hint,omitempty"`
}

// Validate ensures repo metadata is well formed when provided.
func (r RepoMaterialization) Validate() error {
	// URL is optional; when set, validate and require either target ref or commit.
	if r.URL != "" {
		if err := r.URL.Validate(); err != nil {
			return fmt.Errorf("url: %w", err)
		}
		if r.TargetRef == "" && r.Commit == "" {
			return fmt.Errorf("target_ref or commit is required when repo url is set")
		}
	}
	// Validate optional refs/commit when provided.
	if r.BaseRef != "" {
		if err := r.BaseRef.Validate(); err != nil {
			return fmt.Errorf("base_ref: %w", err)
		}
	}
	if r.TargetRef != "" {
		if err := r.TargetRef.Validate(); err != nil {
			return fmt.Errorf("target_ref: %w", err)
		}
	}
	if r.Commit != "" {
		if err := r.Commit.Validate(); err != nil {
			return fmt.Errorf("commit: %w", err)
		}
	}
	return nil
}
