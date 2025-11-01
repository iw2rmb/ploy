package contracts

import (
	"fmt"
	"strings"
)

type WorkflowTicket struct {
	SchemaVersion string              `json:"schema_version"`
	TicketID      string              `json:"ticket_id"`
	Manifest      ManifestReference   `json:"manifest"`
	Repo          RepoMaterialization `json:"repo,omitempty"`
}

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
	if !strings.HasPrefix(strings.TrimSpace(r.URL), "http://") && !strings.HasPrefix(strings.TrimSpace(r.URL), "https://") && !strings.HasPrefix(strings.TrimSpace(r.URL), "git@") {
		return fmt.Errorf("url must be http(s) or ssh")
	}
	if strings.TrimSpace(r.TargetRef) == "" && strings.TrimSpace(r.Commit) == "" {
		return fmt.Errorf("target_ref or commit is required when repo url is set")
	}
	return nil
}
