package migs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestAddModRepoCommand_Run validates AddMigRepoCommand responses.
func TestAddModRepoCommand_Run(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewMigID().String()

	tests := []struct {
		name        string
		modID       string
		repoURL     string
		baseRef     string
		targetRef   string
		statusCode  int
		wantErr     bool
		wantErrText string
	}{
		{
			name:       "successful add",
			modID:      modID,
			repoURL:    "https://github.com/org/repo.git",
			baseRef:    "main",
			targetRef:  "feature-branch",
			statusCode: http.StatusCreated,
		},
		{
			name:        "missing repo url",
			modID:       modID,
			repoURL:     "",
			baseRef:     "main",
			targetRef:   "feature",
			wantErr:     true,
			wantErrText: "repo url is required",
		},
		{
			name:        "missing base ref",
			modID:       modID,
			repoURL:     "https://github.com/org/repo.git",
			baseRef:     "",
			targetRef:   "feature",
			wantErr:     true,
			wantErrText: "base ref is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}

				resp := MigRepoSummary{
					ID:        domaintypes.MigRepoID("repo-001"),
					MigID:     domaintypes.MigID(tc.modID),
					RepoURL:   tc.repoURL,
					BaseRef:   domaintypes.GitRef(tc.baseRef),
					TargetRef: domaintypes.GitRef(tc.targetRef),
					CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_ = json.NewEncoder(w).Encode(resp)
			}))
			t.Cleanup(srv.Close)

			baseURL, _ := url.Parse(srv.URL)

			cmd := AddMigRepoCommand{
				Client:    srv.Client(),
				BaseURL:   baseURL,
				MigRef:    domaintypes.MigRef(tc.modID),
				RepoURL:   tc.repoURL,
				BaseRef:   tc.baseRef,
				TargetRef: tc.targetRef,
			}

			result, err := cmd.Run(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.wantErrText != "" && !strings.Contains(err.Error(), tc.wantErrText) {
					t.Errorf("error %q should contain %q", err.Error(), tc.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			if result.ID.String() != "repo-001" {
				t.Errorf("got ID %q, want %q", result.ID.String(), "repo-001")
			}
		})
	}
}

// TestListModReposCommand_Run validates ListMigReposCommand responses.
func TestListModReposCommand_Run(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewMigID()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		resp := struct {
			Repos []MigRepoSummary `json:"repos"`
		}{
			Repos: []MigRepoSummary{
				{ID: domaintypes.MigRepoID("repo-001"), MigID: modID, RepoURL: "https://github.com/a/b.git", BaseRef: domaintypes.GitRef("main"), TargetRef: domaintypes.GitRef("feat"), CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				{ID: domaintypes.MigRepoID("repo-002"), MigID: modID, RepoURL: "https://github.com/c/d.git", BaseRef: domaintypes.GitRef("main"), TargetRef: domaintypes.GitRef("fix"), CreatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	baseURL, _ := url.Parse(srv.URL)

	cmd := ListMigReposCommand{
		Client:  srv.Client(),
		BaseURL: baseURL,
		MigRef:  domaintypes.MigRef(modID.String()),
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("got %d results, want 2", len(result))
	}
}

// TestRemoveModRepoCommand_Run validates RemoveMigRepoCommand responses.
func TestRemoveModRepoCommand_Run(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewMigID()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	baseURL, _ := url.Parse(srv.URL)

	cmd := RemoveMigRepoCommand{
		Client:  srv.Client(),
		BaseURL: baseURL,
		MigRef:  domaintypes.MigRef(modID.String()),
		RepoID:  domaintypes.MigRepoID("repo-001"),
	}

	err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
}

// TestImportModReposCommand_Run validates ImportMigReposCommand responses.
func TestImportModReposCommand_Run(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewMigID()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/bulk") {
			t.Errorf("expected path to contain /bulk, got %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "text/csv" {
			t.Errorf("expected Content-Type text/csv, got %s", ct)
		}

		resp := ImportMigReposResult{
			Created: 2,
			Updated: 1,
			Failed:  0,
			Errors:  nil,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	baseURL, _ := url.Parse(srv.URL)

	csvData := []byte("repo_url,base_ref,target_ref\nhttps://github.com/a/b.git,main,feat\nhttps://github.com/c/d.git,main,fix\n")

	cmd := ImportMigReposCommand{
		Client:  srv.Client(),
		BaseURL: baseURL,
		MigRef:  domaintypes.MigRef(modID.String()),
		CSVData: csvData,
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Created != 2 {
		t.Errorf("got Created %d, want 2", result.Created)
	}
}
