package mods

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

func TestListRunsForRepoCommand_Success(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/repos":
			if r.URL.Query().Get("contains") == "" {
				t.Errorf("expected contains query param to be set")
			}
			resp := struct {
				Repos []struct {
					RepoID  string `json:"repo_id"`
					RepoURL string `json:"repo_url"`
				} `json:"repos"`
			}{
				Repos: []struct {
					RepoID  string `json:"repo_id"`
					RepoURL string `json:"repo_url"`
				}{
					{RepoID: "repo-1", RepoURL: "https://github.com/org/repo"},
					{RepoID: "repo-2", RepoURL: "https://github.com/org/other"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return

		case r.Method == http.MethodGet && r.URL.Path == "/v1/repos/repo-1/runs":
			if r.URL.Query().Get("limit") != "100" {
				t.Errorf("unexpected limit: got %q, want %q", r.URL.Query().Get("limit"), "100")
			}
			resp := struct {
				Runs []RepoRunSummary `json:"runs"`
			}{
				Runs: []RepoRunSummary{
					{
						RunID:      domaintypes.RunID("run-456"),
						ModID:      "mod-123",
						RunStatus:  "Finished",
						RepoStatus: "Success",
						BaseRef:    "main",
						TargetRef:  "feature-branch",
						Attempt:    1,
						StartedAt:  ptrTime(time.Unix(1, 0).UTC()),
						FinishedAt: ptrTime(time.Unix(2, 0).UTC()),
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return

		default:
			t.Fatalf("unexpected HTTP request: %s %s", r.Method, r.URL.String())
		}
	}))
	t.Cleanup(ts.Close)

	baseURL, _ := url.Parse(ts.URL)
	cmd := ListRunsForRepoCommand{
		Client:  ts.Client(),
		BaseURL: baseURL,
		RepoURL: "https://github.com/org/repo.git",
		Limit:   100,
	}

	runs, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].RunID.String() != "run-456" {
		t.Errorf("unexpected run id: %s", runs[0].RunID.String())
	}
	if runs[0].ModID != "mod-123" {
		t.Errorf("unexpected mod id: %s", runs[0].ModID)
	}
	if runs[0].RepoStatus != "Success" {
		t.Errorf("unexpected repo status: %s", runs[0].RepoStatus)
	}
}

func TestListRunsForRepoCommand_RepoNotFound(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/repos" {
			t.Fatalf("unexpected HTTP request: %s %s", r.Method, r.URL.String())
		}
		resp := struct {
			Repos []struct {
				RepoID  string `json:"repo_id"`
				RepoURL string `json:"repo_url"`
			} `json:"repos"`
		}{Repos: nil}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(ts.Close)

	baseURL, _ := url.Parse(ts.URL)
	cmd := ListRunsForRepoCommand{
		Client:  ts.Client(),
		BaseURL: baseURL,
		RepoURL: "https://github.com/org/repo.git",
	}

	_, err := cmd.Run(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "repo not found") {
		t.Fatalf("expected error to mention repo not found, got %q", err.Error())
	}
}

func TestListRunsForRepoCommand_EmptyRepoURL(t *testing.T) {
	t.Parallel()

	baseURL, _ := url.Parse("http://example.com")
	cmd := ListRunsForRepoCommand{
		Client:  http.DefaultClient,
		BaseURL: baseURL,
		RepoURL: "",
	}

	_, err := cmd.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for empty repo URL")
	}
}

func TestListRunsForRepoCommand_InvalidRepoURLScheme(t *testing.T) {
	t.Parallel()

	baseURL, _ := url.Parse("http://example.com")
	cmd := ListRunsForRepoCommand{
		Client:  http.DefaultClient,
		BaseURL: baseURL,
		RepoURL: "http://github.com/org/repo.git",
	}

	_, err := cmd.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid repo URL scheme")
	}
	if !strings.Contains(err.Error(), "repo_url") {
		t.Fatalf("expected error to mention repo_url, got %q", err.Error())
	}
}

func TestResolveRunForRepo_MatchByRunID(t *testing.T) {
	t.Parallel()

	runs := []RepoRunSummary{
		{RunID: domaintypes.RunID("run-111")},
		{RunID: domaintypes.RunID("run-222")},
	}
	got := ResolveRunForRepo(runs, "run-222")
	if got == nil || got.RunID.String() != "run-222" {
		t.Fatalf("expected run-222, got %#v", got)
	}
}

func TestResolveRunForRepo_TrimsWhitespace(t *testing.T) {
	t.Parallel()

	runs := []RepoRunSummary{{RunID: domaintypes.RunID("run-222")}}
	got := ResolveRunForRepo(runs, "  run-222  ")
	if got == nil || got.RunID.String() != "run-222" {
		t.Fatalf("expected run-222, got %#v", got)
	}
}

func TestResolveRunForRepo_NoMatch(t *testing.T) {
	t.Parallel()

	runs := []RepoRunSummary{{RunID: domaintypes.RunID("run-222")}}
	got := ResolveRunForRepo(runs, "run-999")
	if got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
