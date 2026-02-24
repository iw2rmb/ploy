package migs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestCreateMigRunCommand_Run validates CreateMigRunCommand responses.
func TestCreateMigRunCommand_Run(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewMigID().String()

	tests := []struct {
		name        string
		modID       string
		repoURLs    []string
		failed      bool
		statusCode  int
		wantErr     bool
		wantErrText string
	}{
		{
			name:       "run all repos",
			modID:      modID,
			repoURLs:   nil,
			failed:     false,
			statusCode: http.StatusCreated,
		},
		{
			name:       "run failed repos",
			modID:      modID,
			repoURLs:   nil,
			failed:     true,
			statusCode: http.StatusCreated,
		},
		{
			name:       "run explicit repos",
			modID:      modID,
			repoURLs:   []string{"https://github.com/a/b.git", "https://github.com/c/d.git"},
			failed:     false,
			statusCode: http.StatusCreated,
		},
		{
			name:        "mutually exclusive flags",
			modID:       modID,
			repoURLs:    []string{"https://github.com/a/b.git"},
			failed:      true,
			wantErr:     true,
			wantErrText: "mutually exclusive",
		},
		{
			name:        "missing mig id",
			modID:       "",
			wantErr:     true,
			wantErrText: "mig id is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runID := domaintypes.NewRunID()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if !strings.Contains(r.URL.Path, "/runs") {
					t.Errorf("expected path to contain /runs, got %s", r.URL.Path)
				}

				// Verify request body.
				var req struct {
					RepoSelector struct {
						Mode  string   `json:"mode"`
						Repos []string `json:"repos,omitempty"`
					} `json:"repo_selector"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("decode request body: %v", err)
				}

				// Verify mode based on test case.
				if tc.failed && req.RepoSelector.Mode != "failed" {
					t.Errorf("expected mode failed, got %s", req.RepoSelector.Mode)
				}
				if len(tc.repoURLs) > 0 && !tc.failed && req.RepoSelector.Mode != "explicit" {
					t.Errorf("expected mode explicit, got %s", req.RepoSelector.Mode)
				}
				if len(tc.repoURLs) == 0 && !tc.failed && req.RepoSelector.Mode != "all" {
					t.Errorf("expected mode all, got %s", req.RepoSelector.Mode)
				}

				resp := CreateMigRunResult{RunID: runID}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_ = json.NewEncoder(w).Encode(resp)
			}))
			t.Cleanup(srv.Close)

			baseURL, _ := url.Parse(srv.URL)

			cmd := CreateMigRunCommand{
				Client:   srv.Client(),
				BaseURL:  baseURL,
				MigRef:   domaintypes.MigRef(tc.modID),
				RepoURLs: tc.repoURLs,
				Failed:   tc.failed,
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
			if result.RunID != runID {
				t.Errorf("got RunID %q, want %q", result.RunID.String(), runID.String())
			}
		})
	}
}

// TestCreateMigRunCommand_SelectorMutualExclusion validates --repo and --failed are mutually exclusive.
func TestCreateMigRunCommand_SelectorMutualExclusion(t *testing.T) {
	t.Parallel()

	modID := domaintypes.NewMigID()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP request: %s %s", r.Method, r.URL.String())
	}))
	t.Cleanup(srv.Close)

	baseURL, _ := url.Parse(srv.URL)

	cmd := CreateMigRunCommand{
		Client:   srv.Client(),
		BaseURL:  baseURL,
		MigRef:   domaintypes.MigRef(modID.String()),
		RepoURLs: []string{"https://github.com/org/repo.git"},
		Failed:   true,
	}

	_, err := cmd.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for mutually exclusive flags")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected error to mention mutually exclusive, got %q", err.Error())
	}
}
