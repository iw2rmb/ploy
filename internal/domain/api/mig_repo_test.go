package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestMigRepoSummaryDTO verifies JSON field names and roundtrip stability for MigRepoSummary.
func TestMigRepoSummaryDTO(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	t.Run("roundtrip_full", func(t *testing.T) {
		t.Parallel()
		in := MigRepoSummary{
			ID:        domaintypes.MigRepoID("repoAbCd"),
			MigID:     domaintypes.MigID("migAbc"),
			RepoURL:   "https://github.com/example/repo.git",
			BaseRef:   "main",
			TargetRef: "feature/branch",
			CreatedAt: now,
		}
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var out MigRepoSummary
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if out.ID != in.ID {
			t.Errorf("ID: got %q, want %q", out.ID, in.ID)
		}
		if out.MigID != in.MigID {
			t.Errorf("MigID: got %q, want %q", out.MigID, in.MigID)
		}
		if out.RepoURL != in.RepoURL {
			t.Errorf("RepoURL: got %q, want %q", out.RepoURL, in.RepoURL)
		}
		if out.BaseRef != in.BaseRef {
			t.Errorf("BaseRef: got %q, want %q", out.BaseRef, in.BaseRef)
		}
		if out.TargetRef != in.TargetRef {
			t.Errorf("TargetRef: got %q, want %q", out.TargetRef, in.TargetRef)
		}
		if !out.CreatedAt.Equal(in.CreatedAt) {
			t.Errorf("CreatedAt: got %v, want %v", out.CreatedAt, in.CreatedAt)
		}
	})

	t.Run("wire_field_names", func(t *testing.T) {
		t.Parallel()
		in := MigRepoSummary{
			ID:        domaintypes.MigRepoID("repoAbCd"),
			MigID:     domaintypes.MigID("migAbc"),
			RepoURL:   "https://github.com/example/repo.git",
			BaseRef:   "main",
			TargetRef: "feature",
			CreatedAt: now,
		}
		b, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		js := string(b)
		for _, want := range []string{
			`"id":`, `"mig_id":`, `"repo_url":`, `"base_ref":`, `"target_ref":`, `"created_at":`,
		} {
			if !strings.Contains(js, want) {
				t.Errorf("JSON missing field %s in %s", want, js)
			}
		}
	})
}

// TestMigRepoListResponseDTO verifies the list envelope JSON field name.
func TestMigRepoListResponseDTO(t *testing.T) {
	t.Parallel()

	resp := MigRepoListResponse{
		Repos: []MigRepoSummary{
			{
				ID:        domaintypes.MigRepoID("repoAbCd"),
				MigID:     domaintypes.MigID("migAbc"),
				RepoURL:   "https://github.com/example/repo.git",
				BaseRef:   "main",
				TargetRef: "feature",
				CreatedAt: time.Now(),
			},
		},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"repos":`) {
		t.Errorf("envelope field name must be \"repos\", got %s", string(b))
	}
}
