package contracts

import (
	"fmt"
	"strings"
)

// WorkflowTicket is the envelope used when submitting or claiming a workflow
// run. It carries the schema version, the opaque ticket identifier, the
// manifest reference (name/version), and optional repository materialization
// details for nodes to hydrate workspaces.
type WorkflowTicket struct {
	SchemaVersion string              `json:"schema_version"`
	TicketID      string              `json:"ticket_id"`
	Manifest      ManifestReference   `json:"manifest"`
	Repo          RepoMaterialization `json:"repo,omitempty"`
}

// Validate checks that required fields are present and that embedded
// structures are valid. It requires a non‑empty schema version and ticket ID,
// a valid `Manifest`, and (when provided) a valid `Repo`.
func (t WorkflowTicket) Validate() error {
	if t.SchemaVersion == "" {
		return fmt.Errorf("schema_version is required")
	}
	if t.TicketID == "" {
		return fmt.Errorf("ticket_id is required")
	}
	if err := t.Manifest.Validate(); err != nil {
		return fmt.Errorf("manifest invalid: %w", err)
	}
	if err := t.Repo.Validate(); err != nil {
		return fmt.Errorf("repo invalid: %w", err)
	}
	return nil
}

// RepoMaterialization describes repository inputs required for a workflow run.
type RepoMaterialization struct {
	URL           string `json:"url"`
	BaseRef       string `json:"base_ref"`
	TargetRef     string `json:"target_ref"`
	Commit        string `json:"commit,omitempty"`
	WorkspaceHint string `json:"workspace_hint,omitempty"`
}

// Validate ensures repo metadata is well formed when provided.
func (r RepoMaterialization) Validate() error {
	if strings.TrimSpace(r.URL) == "" {
		return nil
	}
	url := strings.TrimSpace(r.URL)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "git@") && !strings.HasPrefix(url, "file://") {
		return fmt.Errorf("url must be http(s), ssh, or file://")
	}
	if strings.TrimSpace(r.TargetRef) == "" && strings.TrimSpace(r.Commit) == "" {
		return fmt.Errorf("target_ref or commit is required when repo url is set")
	}
	return nil
}
