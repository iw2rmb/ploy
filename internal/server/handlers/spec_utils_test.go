package handlers

import (
	"encoding/json"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/config"
)

// TestMergeGitLabConfigIntoSpec_EmptyConfig verifies that when GitLab config is empty,
// the spec is returned unchanged.
func TestMergeGitLabConfigIntoSpec_EmptyConfig(t *testing.T) {
	t.Parallel()

	spec := json.RawMessage(`{"job_id":"abc123"}`)
	cfg := config.GitLabConfig{}

	result := mergeGitLabConfigIntoSpec(spec, cfg)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if _, hasToken := m["gitlab_pat"]; hasToken {
		t.Error("gitlab_pat should not be present when config is empty")
	}
	if _, hasDomain := m["gitlab_domain"]; hasDomain {
		t.Error("gitlab_domain should not be present when config is empty")
	}
	if m["job_id"] != "abc123" {
		t.Errorf("job_id = %v, want abc123", m["job_id"])
	}
}

// TestMergeGitLabConfigIntoSpec_ServerDefaults verifies that server defaults
// are merged into spec when spec does not contain per-run overrides.
func TestMergeGitLabConfigIntoSpec_ServerDefaults(t *testing.T) {
	t.Parallel()

	spec := json.RawMessage(`{"job_id":"abc123"}`)
	cfg := config.GitLabConfig{
		Domain: "https://gitlab.example.com",
		Token:  "server-token-default",
	}

	result := mergeGitLabConfigIntoSpec(spec, cfg)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if m["gitlab_pat"] != "server-token-default" {
		t.Errorf("gitlab_pat = %v, want server-token-default", m["gitlab_pat"])
	}
	if m["gitlab_domain"] != "https://gitlab.example.com" {
		t.Errorf("gitlab_domain = %v, want https://gitlab.example.com", m["gitlab_domain"])
	}
	if m["job_id"] != "abc123" {
		t.Errorf("job_id = %v, want abc123", m["job_id"])
	}
}

// TestMergeGitLabConfigIntoSpec_PerRunOverrides verifies that per-run overrides
// in spec take precedence over server defaults.
func TestMergeGitLabConfigIntoSpec_PerRunOverrides(t *testing.T) {
	t.Parallel()

	spec := json.RawMessage(`{
		"job_id":"abc123",
		"gitlab_pat":"per-run-token",
		"gitlab_domain":"https://gitlab.custom.com"
	}`)
	cfg := config.GitLabConfig{
		Domain: "https://gitlab.example.com",
		Token:  "server-token-default",
	}

	result := mergeGitLabConfigIntoSpec(spec, cfg)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Per-run overrides should be preserved.
	if m["gitlab_pat"] != "per-run-token" {
		t.Errorf("gitlab_pat = %v, want per-run-token", m["gitlab_pat"])
	}
	if m["gitlab_domain"] != "https://gitlab.custom.com" {
		t.Errorf("gitlab_domain = %v, want https://gitlab.custom.com", m["gitlab_domain"])
	}
	if m["job_id"] != "abc123" {
		t.Errorf("job_id = %v, want abc123", m["job_id"])
	}
}

// TestMergeGitLabConfigIntoSpec_PartialOverrides verifies that if only one field
// has a per-run override, the other field gets the server default.
func TestMergeGitLabConfigIntoSpec_PartialOverrides(t *testing.T) {
	t.Parallel()

	spec := json.RawMessage(`{
		"job_id":"abc123",
		"gitlab_pat":"per-run-token"
	}`)
	cfg := config.GitLabConfig{
		Domain: "https://gitlab.example.com",
		Token:  "server-token-default",
	}

	result := mergeGitLabConfigIntoSpec(spec, cfg)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// Per-run token should be preserved.
	if m["gitlab_pat"] != "per-run-token" {
		t.Errorf("gitlab_pat = %v, want per-run-token", m["gitlab_pat"])
	}
	// Server default domain should be added.
	if m["gitlab_domain"] != "https://gitlab.example.com" {
		t.Errorf("gitlab_domain = %v, want https://gitlab.example.com", m["gitlab_domain"])
	}
}

// TestMergeGitLabConfigIntoSpec_EmptySpec verifies that merging works even when
// spec is empty or nil.
func TestMergeGitLabConfigIntoSpec_EmptySpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec json.RawMessage
	}{
		{"nil spec", nil},
		{"empty spec", json.RawMessage(`{}`)},
		{"whitespace spec", json.RawMessage(`   `)},
	}

	cfg := config.GitLabConfig{
		Domain: "https://gitlab.example.com",
		Token:  "server-token-default",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeGitLabConfigIntoSpec(tt.spec, cfg)

			var m map[string]any
			if err := json.Unmarshal(result, &m); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}

			if m["gitlab_pat"] != "server-token-default" {
				t.Errorf("gitlab_pat = %v, want server-token-default", m["gitlab_pat"])
			}
			if m["gitlab_domain"] != "https://gitlab.example.com" {
				t.Errorf("gitlab_domain = %v, want https://gitlab.example.com", m["gitlab_domain"])
			}
		})
	}
}

// TestMergeGitLabConfigIntoSpec_OnlyToken verifies that only token is merged
// when domain is empty in config.
func TestMergeGitLabConfigIntoSpec_OnlyToken(t *testing.T) {
	t.Parallel()

	spec := json.RawMessage(`{"job_id":"abc123"}`)
	cfg := config.GitLabConfig{
		Token: "server-token-only",
	}

	result := mergeGitLabConfigIntoSpec(spec, cfg)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if m["gitlab_pat"] != "server-token-only" {
		t.Errorf("gitlab_pat = %v, want server-token-only", m["gitlab_pat"])
	}
	if _, hasDomain := m["gitlab_domain"]; hasDomain {
		t.Error("gitlab_domain should not be present when config domain is empty")
	}
}

// TestMergeGitLabConfigIntoSpec_OnlyDomain verifies that only domain is merged
// when token is empty in config.
func TestMergeGitLabConfigIntoSpec_OnlyDomain(t *testing.T) {
	t.Parallel()

	spec := json.RawMessage(`{"job_id":"abc123"}`)
	cfg := config.GitLabConfig{
		Domain: "https://gitlab.example.com",
	}

	result := mergeGitLabConfigIntoSpec(spec, cfg)

	var m map[string]any
	if err := json.Unmarshal(result, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if _, hasToken := m["gitlab_pat"]; hasToken {
		t.Error("gitlab_pat should not be present when config token is empty")
	}
	if m["gitlab_domain"] != "https://gitlab.example.com" {
		t.Errorf("gitlab_domain = %v, want https://gitlab.example.com", m["gitlab_domain"])
	}
}
