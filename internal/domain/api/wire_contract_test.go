package api

import (
	"encoding/json"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestJSONWireContracts(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	specID := domaintypes.SpecID("specAbCd")
	createdBy := "ci-bot"
	username := "alice"
	description := "automation token"
	lastUsedAt := now.Add(time.Hour)
	mig := MigSummary{
		ID:        domaintypes.MigID("migAbc"),
		Name:      "my-mig",
		SpecID:    &specID,
		CreatedBy: &createdBy,
		Archived:  true,
		CreatedAt: now,
	}
	migRepo := MigRepoSummary{
		ID:        domaintypes.MigRepoID("repoAbCd"),
		MigID:     domaintypes.MigID("migAbc"),
		RepoURL:   "https://github.com/example/repo.git",
		BaseRef:   "main",
		CreatedAt: now,
	}

	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			name:  "mig summary exports optional fields",
			value: mig,
			want:  `{"id":"migAbc","name":"my-mig","spec_id":"specAbCd","created_by":"ci-bot","archived":true,"created_at":"2025-06-01T12:00:00Z"}`,
		},
		{
			name: "mig summary omits absent optional fields",
			value: MigSummary{
				ID: domaintypes.MigID("migAbc"), Name: "my-mig", CreatedAt: now,
			},
			want: `{"id":"migAbc","name":"my-mig","archived":false,"created_at":"2025-06-01T12:00:00Z"}`,
		},
		{
			name:  "mig list envelope",
			value: MigListResponse{Migs: []MigSummary{mig}},
			want:  `{"migs":[{"id":"migAbc","name":"my-mig","spec_id":"specAbCd","created_by":"ci-bot","archived":true,"created_at":"2025-06-01T12:00:00Z"}]}`,
		},
		{
			name:  "mig repo summary",
			value: migRepo,
			want:  `{"id":"repoAbCd","mig_id":"migAbc","repo_url":"https://github.com/example/repo.git","base_ref":"main","created_at":"2025-06-01T12:00:00Z"}`,
		},
		{
			name:  "mig repo list envelope",
			value: MigRepoListResponse{Repos: []MigRepoSummary{migRepo}},
			want:  `{"repos":[{"id":"repoAbCd","mig_id":"migAbc","repo_url":"https://github.com/example/repo.git","base_ref":"main","created_at":"2025-06-01T12:00:00Z"}]}`,
		},
		{
			name: "mig repo import response",
			value: MigRepoImportResponse{
				Created: 2,
				Updated: 1,
				Failed:  1,
				Errors:  []MigRepoImportError{{Line: 4, Message: "base_ref is required"}},
			},
			want: `{"created":2,"updated":1,"failed":1,"errors":[{"line":4,"message":"base_ref is required"}]}`,
		},
		{
			name: "create api token request",
			value: CreateAPITokenRequest{
				Role: "control-plane", Username: username, Description: description, ExpiresInDays: 365,
			},
			want: `{"role":"control-plane","username":"alice","description":"automation token","expires_in_days":365}`,
		},
		{
			name:  "create api token request omits absent optional fields",
			value: CreateAPITokenRequest{Role: "worker", ExpiresInDays: 30},
			want:  `{"role":"worker","expires_in_days":30}`,
		},
		{
			name: "create api token response",
			value: CreateAPITokenResponse{
				Token: "secret", TokenID: "token-1", Role: "control-plane", Username: &username,
				ExpiresAt: now, Warning: "save it",
			},
			want: `{"token":"secret","token_id":"token-1","role":"control-plane","username":"alice","expires_at":"2025-06-01T12:00:00Z","warning":"save it"}`,
		},
		{
			name: "create api token response omits absent username",
			value: CreateAPITokenResponse{
				Token: "secret", TokenID: "token-2", Role: "worker", ExpiresAt: now, Warning: "save it",
			},
			want: `{"token":"secret","token_id":"token-2","role":"worker","expires_at":"2025-06-01T12:00:00Z","warning":"save it"}`,
		},
		{
			name: "api token list response",
			value: APITokenListResponse{Tokens: []APITokenListItem{{
				TokenID: "token-1", Role: "control-plane", Username: &username, Description: &description,
				IssuedAt: now, ExpiresAt: now.Add(24 * time.Hour), LastUsedAt: &lastUsedAt, CreatedBy: &createdBy,
			}}},
			want: `{"tokens":[{"token_id":"token-1","role":"control-plane","username":"alice","description":"automation token","issued_at":"2025-06-01T12:00:00Z","expires_at":"2025-06-02T12:00:00Z","last_used_at":"2025-06-01T13:00:00Z","created_by":"ci-bot"}]}`,
		},
		{
			name:  "create bootstrap token request",
			value: CreateBootstrapTokenRequest{NodeID: "nodeAbCd", ExpiresInMinutes: 15},
			want:  `{"node_id":"nodeAbCd","expires_in_minutes":15}`,
		},
		{
			name:  "create bootstrap token response",
			value: CreateBootstrapTokenResponse{Token: "secret", NodeID: "nodeAbCd", ExpiresAt: now},
			want:  `{"token":"secret","node_id":"nodeAbCd","expires_at":"2025-06-01T12:00:00Z"}`,
		},
		{
			name: "node heartbeat request",
			value: NodeHeartbeatRequest{
				CPUFreeMillis: 1500, CPUTotalMillis: 4000,
				MemFreeBytes: 2147483648, MemTotalBytes: 8589934592,
				DiskFreeBytes: 10737418240, DiskTotalBytes: 53687091200,
				Version: "ployd-node/test",
			},
			want: `{"cpu_free_millis":1500,"cpu_total_millis":4000,"mem_free_bytes":2147483648,"mem_total_bytes":8589934592,"disk_free_bytes":10737418240,"disk_total_bytes":53687091200,"version":"ployd-node/test"}`,
		},
		{
			name: "run submit exports populated fields",
			value: RunSubmitRequest{
				RepoURL: domaintypes.RepoURL("https://github.com/example/repo.git"),
				Ref:     domaintypes.GitRef("main"), Spec: json.RawMessage(`{"key":"value"}`), CreatedBy: "ci-bot",
			},
			want: `{"repo_url":"https://github.com/example/repo.git","ref":"main","spec":{"key":"value"},"created_by":"ci-bot"}`,
		},
		{
			name: "run submit omits absent optional fields",
			value: RunSubmitRequest{
				RepoURL: domaintypes.RepoURL("https://github.com/example/repo.git"),
				Ref:     domaintypes.GitRef("main"), Spec: json.RawMessage(`{}`),
			},
			want: `{"repo_url":"https://github.com/example/repo.git","ref":"main","spec":{}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("json.Marshal() = %s, want %s", got, tt.want)
			}
		})
	}
}
