package handlers

import (
	"encoding/json"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/config"
)

func TestApplyGitLabConfigMutator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		spec       json.RawMessage
		cfg        config.GitLabConfig
		wantErr    bool
		wantToken  string // "" means absent
		wantDomain string // "" means absent
		checkJobID string // if non-empty, check job_id preserved
	}{
		{
			name:       "empty config leaves spec unchanged",
			spec:       json.RawMessage(`{"job_id":"abc123"}`),
			cfg:        config.GitLabConfig{},
			wantToken:  "",
			wantDomain: "",
			checkJobID: "abc123",
		},
		{
			name:       "server defaults merged when spec has no overrides",
			spec:       json.RawMessage(`{"job_id":"abc123"}`),
			cfg:        config.GitLabConfig{Domain: "https://gitlab.example.com", Token: "server-token-default"},
			wantToken:  "server-token-default",
			wantDomain: "https://gitlab.example.com",
			checkJobID: "abc123",
		},
		{
			name: "per-run overrides take precedence",
			spec: json.RawMessage(`{"job_id":"abc123","gitlab_pat":"per-run-token","gitlab_domain":"https://gitlab.custom.com"}`),
			cfg:  config.GitLabConfig{Domain: "https://gitlab.example.com", Token: "server-token-default"},

			wantToken:  "per-run-token",
			wantDomain: "https://gitlab.custom.com",
			checkJobID: "abc123",
		},
		{
			name:       "partial override: per-run token preserved, server domain added",
			spec:       json.RawMessage(`{"job_id":"abc123","gitlab_pat":"per-run-token"}`),
			cfg:        config.GitLabConfig{Domain: "https://gitlab.example.com", Token: "server-token-default"},
			wantToken:  "per-run-token",
			wantDomain: "https://gitlab.example.com",
		},
		{
			name:       "only token in config",
			spec:       json.RawMessage(`{"job_id":"abc123"}`),
			cfg:        config.GitLabConfig{Token: "server-token-only"},
			wantToken:  "server-token-only",
			wantDomain: "",
		},
		{
			name:       "only domain in config",
			spec:       json.RawMessage(`{"job_id":"abc123"}`),
			cfg:        config.GitLabConfig{Domain: "https://gitlab.example.com"},
			wantToken:  "",
			wantDomain: "https://gitlab.example.com",
		},
		{
			name:    "nil spec",
			spec:    nil,
			cfg:     config.GitLabConfig{Domain: "d", Token: "t"},
			wantErr: true,
		},
		{
			name:    "whitespace spec",
			spec:    json.RawMessage(`   `),
			cfg:     config.GitLabConfig{Domain: "d", Token: "t"},
			wantErr: true,
		},
		{
			name:    "invalid json",
			spec:    json.RawMessage(`{invalid`),
			cfg:     config.GitLabConfig{Domain: "d", Token: "t"},
			wantErr: true,
		},
		{
			name:    "non-object json",
			spec:    json.RawMessage(`[]`),
			cfg:     config.GitLabConfig{Domain: "d", Token: "t"},
			wantErr: true,
		},
		{
			name:       "empty spec object",
			spec:       json.RawMessage(`{}`),
			cfg:        config.GitLabConfig{Domain: "https://gitlab.example.com", Token: "server-token-default"},
			wantToken:  "server-token-default",
			wantDomain: "https://gitlab.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m, err := parseSpecObjectStrict(tt.spec)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSpecObjectStrict: %v", err)
			}

			if err := applyGitLabConfigMutator(m, tt.cfg); err != nil {
				t.Fatalf("applyGitLabConfigMutator: %v", err)
			}

			result, err := marshalSpecObject(m)
			if err != nil {
				t.Fatalf("marshalSpecObject: %v", err)
			}

			var out map[string]any
			if err := json.Unmarshal(result, &out); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}

			if tt.wantToken != "" {
				if out["gitlab_pat"] != tt.wantToken {
					t.Errorf("gitlab_pat = %v, want %s", out["gitlab_pat"], tt.wantToken)
				}
			} else {
				if _, has := out["gitlab_pat"]; has {
					t.Error("gitlab_pat should not be present")
				}
			}

			if tt.wantDomain != "" {
				if out["gitlab_domain"] != tt.wantDomain {
					t.Errorf("gitlab_domain = %v, want %s", out["gitlab_domain"], tt.wantDomain)
				}
			} else {
				if _, has := out["gitlab_domain"]; has {
					t.Error("gitlab_domain should not be present")
				}
			}

			if tt.checkJobID != "" {
				if out["job_id"] != tt.checkJobID {
					t.Errorf("job_id = %v, want %s", out["job_id"], tt.checkJobID)
				}
			}
		})
	}
}
