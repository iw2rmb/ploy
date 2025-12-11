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

// TestListRunsForRepoCommand_Success tests the ListRunsForRepoCommand happy path.
func TestListRunsForRepoCommand_Success(t *testing.T) {
	t.Parallel()

	// Create a test server that returns a canned response.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request path contains the repo URL (URL-encoded via PathEscape).
		// Note: When url.JoinPath encodes the path, the server receives it as-is,
		// but http.Request.URL.Path is percent-decoded. We check the raw path instead.
		// The path should have the format: /v1/repos/{encoded-repo-url}/runs
		if !strings.HasSuffix(r.URL.Path, "/runs") {
			t.Errorf("unexpected path suffix: got %q, expected to end with /runs", r.URL.Path)
		}
		if !strings.Contains(r.URL.Path, "/v1/repos/") {
			t.Errorf("unexpected path: got %q, expected to contain /v1/repos/", r.URL.Path)
		}
		// Verify the repo URL is embedded (may be URL-decoded in URL.Path).
		if !strings.Contains(r.URL.Path, "github.com") {
			t.Errorf("unexpected path: got %q, expected to contain github.com", r.URL.Path)
		}

		// Verify query parameters.
		if r.URL.Query().Get("limit") != "100" {
			t.Errorf("unexpected limit: got %q, want %q", r.URL.Query().Get("limit"), "100")
		}

		// Return a canned response.
		name := "test-batch"
		execID := "exec-123"
		resp := struct {
			Runs []RepoRunSummary `json:"runs"`
		}{
			Runs: []RepoRunSummary{
				{
					RunID:          domaintypes.RunID("run-456"),
					Name:           &name,
					RunStatus:      "succeeded",
					RepoStatus:     "succeeded",
					BaseRef:        "main",
					TargetRef:      "feature-branch",
					Attempt:        1,
					ExecutionRunID: &execID,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

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

	run := runs[0]
	if run.RunID.String() != "run-456" {
		t.Errorf("unexpected run ID: %s", run.RunID.String())
	}
	if run.Name == nil || *run.Name != "test-batch" {
		t.Errorf("unexpected name: %v", run.Name)
	}
	if run.RepoStatus != "succeeded" {
		t.Errorf("unexpected repo status: %s", run.RepoStatus)
	}
	if run.ExecutionRunID == nil || *run.ExecutionRunID != "exec-123" {
		t.Errorf("unexpected execution run ID: %v", run.ExecutionRunID)
	}
}

// TestListRunsForRepoCommand_EmptyRepoURL tests that empty repo URL returns an error.
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

// TestListRunsForRepoCommand_HTTPError tests error handling for non-200 responses.
func TestListRunsForRepoCommand_HTTPError(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer ts.Close()

	baseURL, _ := url.Parse(ts.URL)
	cmd := ListRunsForRepoCommand{
		Client:  ts.Client(),
		BaseURL: baseURL,
		RepoURL: "https://github.com/org/repo.git",
	}

	_, err := cmd.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for HTTP 404")
	}
}

// TestResolveRunForRepo_MatchByRunID tests that run resolution prefers RunID match.
func TestResolveRunForRepo_MatchByRunID(t *testing.T) {
	t.Parallel()

	name1 := "batch-1"
	name2 := "batch-2"
	runs := []RepoRunSummary{
		{
			RunID:      domaintypes.RunID("run-111"),
			Name:       &name1,
			RepoStatus: "succeeded",
		},
		{
			RunID:      domaintypes.RunID("run-222"),
			Name:       &name2,
			RepoStatus: "succeeded",
		},
	}

	// Match by RunID should select run-222 even though batch-1 is listed first.
	resolved := ResolveRunForRepo(runs, "run-222")
	if resolved == nil {
		t.Fatal("expected to resolve run by ID")
	}
	if resolved.RunID.String() != "run-222" {
		t.Errorf("expected run-222, got %s", resolved.RunID.String())
	}
}

// TestResolveRunForRepo_MatchByName tests that run resolution falls back to Name match.
func TestResolveRunForRepo_MatchByName(t *testing.T) {
	t.Parallel()

	name1 := "batch-1"
	name2 := "java17-fleet"
	runs := []RepoRunSummary{
		{
			RunID:      domaintypes.RunID("run-111"),
			Name:       &name1,
			RepoStatus: "succeeded",
		},
		{
			RunID:      domaintypes.RunID("run-222"),
			Name:       &name2,
			RepoStatus: "succeeded",
		},
	}

	// Match by Name should select run-222.
	resolved := ResolveRunForRepo(runs, "java17-fleet")
	if resolved == nil {
		t.Fatal("expected to resolve run by name")
	}
	if resolved.RunID.String() != "run-222" {
		t.Errorf("expected run-222, got %s", resolved.RunID.String())
	}
}

// TestResolveRunForRepo_SelectFirstNameMatch tests that the first matching name is selected.
func TestResolveRunForRepo_SelectFirstNameMatch(t *testing.T) {
	t.Parallel()

	// Multiple runs with the same name; API returns DESC by created_at so first entry is newest.
	name := "java17-fleet"
	runs := []RepoRunSummary{
		{
			RunID:      domaintypes.RunID("run-333"), // First/newest entry
			Name:       &name,
			RepoStatus: "succeeded",
		},
		{
			RunID:      domaintypes.RunID("run-222"), // Second/older entry
			Name:       &name,
			RepoStatus: "succeeded",
		},
	}

	// Per ROADMAP.md: select first matching result when multiple share the same name.
	resolved := ResolveRunForRepo(runs, "java17-fleet")
	if resolved == nil {
		t.Fatal("expected to resolve run by name")
	}
	if resolved.RunID.String() != "run-333" {
		t.Errorf("expected run-333 (first match), got %s", resolved.RunID.String())
	}
}

// TestResolveRunForRepo_FilterNonTerminalStatuses tests that only terminal statuses are matched.
func TestResolveRunForRepo_FilterNonTerminalStatuses(t *testing.T) {
	t.Parallel()

	name := "my-batch"
	runs := []RepoRunSummary{
		{
			RunID:      domaintypes.RunID("run-pending"),
			Name:       &name,
			RepoStatus: "pending", // Should be filtered out.
		},
		{
			RunID:      domaintypes.RunID("run-running"),
			Name:       &name,
			RepoStatus: "running", // Should be filtered out.
		},
		{
			RunID:      domaintypes.RunID("run-succeeded"),
			Name:       &name,
			RepoStatus: "succeeded", // Should be matched.
		},
	}

	resolved := ResolveRunForRepo(runs, "my-batch")
	if resolved == nil {
		t.Fatal("expected to resolve run")
	}
	// Only run-succeeded has a terminal status.
	if resolved.RunID.String() != "run-succeeded" {
		t.Errorf("expected run-succeeded, got %s", resolved.RunID.String())
	}
}

// TestResolveRunForRepo_IncludesFailedAndSkipped tests that failed and skipped statuses are matched.
func TestResolveRunForRepo_IncludesFailedAndSkipped(t *testing.T) {
	t.Parallel()

	name := "my-batch"
	tests := []struct {
		status string
	}{
		{"succeeded"},
		{"failed"},
		{"skipped"},
	}

	for _, tc := range tests {
		t.Run(tc.status, func(t *testing.T) {
			runs := []RepoRunSummary{
				{
					RunID:      domaintypes.RunID("run-" + tc.status),
					Name:       &name,
					RepoStatus: tc.status,
				},
			}

			resolved := ResolveRunForRepo(runs, "my-batch")
			if resolved == nil {
				t.Fatalf("expected to resolve run with status %q", tc.status)
			}
		})
	}
}

// TestResolveRunForRepo_NoMatch tests that nil is returned when no match is found.
func TestResolveRunForRepo_NoMatch(t *testing.T) {
	t.Parallel()

	name := "other-batch"
	runs := []RepoRunSummary{
		{
			RunID:      domaintypes.RunID("run-111"),
			Name:       &name,
			RepoStatus: "succeeded",
		},
	}

	resolved := ResolveRunForRepo(runs, "nonexistent")
	if resolved != nil {
		t.Error("expected nil for no match")
	}
}

// TestResolveRunForRepo_EmptyInput tests edge cases with empty inputs.
func TestResolveRunForRepo_EmptyInput(t *testing.T) {
	t.Parallel()

	// Empty runs slice.
	resolved := ResolveRunForRepo(nil, "my-run")
	if resolved != nil {
		t.Error("expected nil for empty runs")
	}

	// Empty run name or ID.
	name := "my-batch"
	runs := []RepoRunSummary{
		{
			RunID:      domaintypes.RunID("run-111"),
			Name:       &name,
			RepoStatus: "succeeded",
		},
	}

	resolved = ResolveRunForRepo(runs, "")
	if resolved != nil {
		t.Error("expected nil for empty runNameOrID")
	}

	resolved = ResolveRunForRepo(runs, "   ")
	if resolved != nil {
		t.Error("expected nil for whitespace-only runNameOrID")
	}
}

// TestResolveRunForRepo_TrimsWhitespace tests that whitespace is trimmed from name matching.
func TestResolveRunForRepo_TrimsWhitespace(t *testing.T) {
	t.Parallel()

	name := "java17-fleet"
	runs := []RepoRunSummary{
		{
			RunID:      domaintypes.RunID("run-111"),
			Name:       &name,
			RepoStatus: "succeeded",
		},
	}

	// Should match even with leading/trailing whitespace.
	resolved := ResolveRunForRepo(runs, "  java17-fleet  ")
	if resolved == nil {
		t.Fatal("expected to resolve run with whitespace-trimmed name")
	}
	if resolved.RunID.String() != "run-111" {
		t.Errorf("expected run-111, got %s", resolved.RunID.String())
	}
}

// TestResolveRunForRepo_RunIDPreferredOverName tests that RunID match takes precedence.
func TestResolveRunForRepo_RunIDPreferredOverName(t *testing.T) {
	t.Parallel()

	// Create a scenario where "run-222" is both a RunID and a Name.
	name1 := "run-222" // Name that looks like a run ID
	name2 := "other-batch"
	runs := []RepoRunSummary{
		{
			RunID:      domaintypes.RunID("run-111"),
			Name:       &name1, // Name is "run-222"
			RepoStatus: "succeeded",
		},
		{
			RunID:      domaintypes.RunID("run-222"), // RunID is "run-222"
			Name:       &name2,
			RepoStatus: "succeeded",
		},
	}

	// Searching for "run-222" should prefer RunID match.
	resolved := ResolveRunForRepo(runs, "run-222")
	if resolved == nil {
		t.Fatal("expected to resolve run")
	}
	// Should match run-222 by ID, not run-111 by name.
	if resolved.RunID.String() != "run-222" {
		t.Errorf("expected run-222 (ID match), got %s", resolved.RunID.String())
	}
}

// TestListRunsForRepoCommand_DefaultLimit tests that limit defaults to 100.
func TestListRunsForRepoCommand_DefaultLimit(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify default limit is applied.
		if r.URL.Query().Get("limit") != "100" {
			t.Errorf("unexpected default limit: got %q, want %q", r.URL.Query().Get("limit"), "100")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Runs []RepoRunSummary `json:"runs"`
		}{Runs: []RepoRunSummary{}})
	}))
	defer ts.Close()

	baseURL, _ := url.Parse(ts.URL)
	cmd := ListRunsForRepoCommand{
		Client:  ts.Client(),
		BaseURL: baseURL,
		RepoURL: "https://github.com/org/repo.git",
		Limit:   0, // Zero should use default of 100.
	}

	_, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestListRunsForRepoCommand_WithOffset tests that offset is passed correctly.
func TestListRunsForRepoCommand_WithOffset(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("offset") != "50" {
			t.Errorf("unexpected offset: got %q, want %q", r.URL.Query().Get("offset"), "50")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Runs []RepoRunSummary `json:"runs"`
		}{Runs: []RepoRunSummary{}})
	}))
	defer ts.Close()

	baseURL, _ := url.Parse(ts.URL)
	cmd := ListRunsForRepoCommand{
		Client:  ts.Client(),
		BaseURL: baseURL,
		RepoURL: "https://github.com/org/repo.git",
		Limit:   100,
		Offset:  50,
	}

	_, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestListRunsForRepoCommand_ParsesAllFields tests that all response fields are parsed.
func TestListRunsForRepoCommand_ParsesAllFields(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := "full-batch"
		execID := "exec-full"
		resp := struct {
			Runs []RepoRunSummary `json:"runs"`
		}{
			Runs: []RepoRunSummary{
				{
					RunID:          "run-full",
					Name:           &name,
					RunStatus:      "running",
					RepoStatus:     "running",
					BaseRef:        "develop",
					TargetRef:      "feature/new",
					Attempt:        2,
					StartedAt:      &now,
					FinishedAt:     nil,
					ExecutionRunID: &execID,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	baseURL, _ := url.Parse(ts.URL)
	cmd := ListRunsForRepoCommand{
		Client:  ts.Client(),
		BaseURL: baseURL,
		RepoURL: "https://github.com/org/repo.git",
	}

	runs, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}

	run := runs[0]
	if run.RunID.String() != "run-full" {
		t.Errorf("unexpected RunID: %s", run.RunID.String())
	}
	if run.Name == nil || *run.Name != "full-batch" {
		t.Errorf("unexpected Name: %v", run.Name)
	}
	if run.RunStatus != "running" {
		t.Errorf("unexpected RunStatus: %s", run.RunStatus)
	}
	if run.RepoStatus != "running" {
		t.Errorf("unexpected RepoStatus: %s", run.RepoStatus)
	}
	if run.BaseRef != "develop" {
		t.Errorf("unexpected BaseRef: %s", run.BaseRef)
	}
	if run.TargetRef != "feature/new" {
		t.Errorf("unexpected TargetRef: %s", run.TargetRef)
	}
	if run.Attempt != 2 {
		t.Errorf("unexpected Attempt: %d", run.Attempt)
	}
	if run.StartedAt == nil {
		t.Error("expected StartedAt to be set")
	}
	if run.FinishedAt != nil {
		t.Error("expected FinishedAt to be nil")
	}
	if run.ExecutionRunID == nil || *run.ExecutionRunID != "exec-full" {
		t.Errorf("unexpected ExecutionRunID: %v", run.ExecutionRunID)
	}
}
