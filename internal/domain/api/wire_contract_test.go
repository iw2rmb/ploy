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
