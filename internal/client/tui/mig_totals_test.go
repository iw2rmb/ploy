package tui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

type migCountTestCase struct {
	name      string
	migID     domaintypes.MigID
	handler   http.HandlerFunc
	wantErr   bool
	wantCount int
}

type migCountCommand func(context.Context, *http.Client, *url.URL, domaintypes.MigID) (int, error)

func runMigCountTestCases(t *testing.T, command migCountCommand, tests []migCountTestCase) {
	t.Helper()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := tc.handler
			if handler == nil {
				handler = func(http.ResponseWriter, *http.Request) {}
			}
			srv := httptest.NewServer(handler)
			t.Cleanup(srv.Close)

			baseURL, _ := url.Parse(srv.URL)
			count, err := command(context.Background(), srv.Client(), baseURL, tc.migID)
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got err=%v", tc.wantErr, err)
			}
			if !tc.wantErr && count != tc.wantCount {
				t.Fatalf("got count=%d, want %d", count, tc.wantCount)
			}
		})
	}
}

func TestCountMigReposCommand(t *testing.T) {
	t.Parallel()

	migID := domaintypes.NewMigID()
	repoID1 := domaintypes.NewMigRepoID()
	repoID2 := domaintypes.NewMigRepoID()

	tests := []migCountTestCase{
		{
			name:  "success returns repo count",
			migID: migID,
			handler: func(w http.ResponseWriter, r *http.Request) {
				wantPath := "/v1/migs/" + migID.String() + "/repos"
				if r.URL.Path != wantPath {
					t.Errorf("path=%q, want %q", r.URL.Path, wantPath)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"repos": []map[string]any{
						{"id": repoID1.String(), "mig_id": migID.String(), "repo_url": "https://github.com/org/a", "base_ref": "main", "created_at": "2026-01-01T00:00:00Z"},
						{"id": repoID2.String(), "mig_id": migID.String(), "repo_url": "https://github.com/org/b", "base_ref": "main", "created_at": "2026-01-01T00:00:00Z"},
					},
				})
			},
			wantCount: 2,
		},
		{
			name:  "success empty repo set returns zero",
			migID: migID,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"repos": []any{}})
			},
			wantCount: 0,
		},
		{
			name:  "http error returns error",
			migID: migID,
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			},
			wantErr: true,
		},
		{
			name:    "zero mig id returns error",
			migID:   domaintypes.MigID(""),
			wantErr: true,
		},
	}

	runMigCountTestCases(t, func(ctx context.Context, client *http.Client, baseURL *url.URL, migID domaintypes.MigID) (int, error) {
		return (CountMigReposCommand{Client: client, BaseURL: baseURL, MigID: migID}).Run(ctx)
	}, tests)
}

func TestCountMigRunsCommand(t *testing.T) {
	t.Parallel()

	migID := domaintypes.NewMigID()
	otherMigID := domaintypes.NewMigID()
	runID1 := domaintypes.NewRunID()
	runID2 := domaintypes.NewRunID()
	runID3 := domaintypes.NewRunID()
	specID := domaintypes.NewSpecID()

	makeRun := func(id domaintypes.RunID, mid domaintypes.MigID) map[string]any {
		return map[string]any{
			"id":         id.String(),
			"status":     "Success",
			"mig_id":     mid.String(),
			"spec_id":    specID.String(),
			"created_at": time.Now().Format(time.RFC3339),
		}
	}

	tests := []migCountTestCase{
		{
			name:  "success counts only matching mig runs",
			migID: migID,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"runs": []map[string]any{
						makeRun(runID1, migID),
						makeRun(runID2, otherMigID), // different mig — should not count
						makeRun(runID3, migID),
					},
				})
			},
			wantCount: 2,
		},
		{
			name:  "success no matching runs returns zero",
			migID: migID,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"runs": []map[string]any{
						makeRun(runID1, otherMigID),
					},
				})
			},
			wantCount: 0,
		},
		{
			name:  "success empty runs list returns zero",
			migID: migID,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"runs": []any{}})
			},
			wantCount: 0,
		},
		{
			name:  "http error returns error",
			migID: migID,
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			},
			wantErr: true,
		},
		{
			name:    "zero mig id returns error",
			migID:   domaintypes.MigID(""),
			wantErr: true,
		},
	}

	runMigCountTestCases(t, func(ctx context.Context, client *http.Client, baseURL *url.URL, migID domaintypes.MigID) (int, error) {
		return (CountMigRunsCommand{Client: client, BaseURL: baseURL, MigID: migID}).Run(ctx)
	}, tests)
}

func TestCountMigRunsCommand_PaginatesAllPages(t *testing.T) {
	t.Parallel()

	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()

	// Build 250 runs: 200 in page 1, 50 in page 2; half belong to migID.
	makeRuns := func(n int, half bool) []map[string]any {
		runs := make([]map[string]any, n)
		for i := range runs {
			mid := migID
			if half && i%2 == 1 {
				mid = domaintypes.NewMigID()
			}
			runs[i] = map[string]any{
				"id":         domaintypes.NewRunID().String(),
				"status":     "Success",
				"mig_id":     mid.String(),
				"spec_id":    specID.String(),
				"created_at": time.Now().Format(time.RFC3339),
			}
		}
		return runs
	}

	page1 := makeRuns(100, false) // 100 matching runs (full page)
	page2 := makeRuns(30, false)  // 30 matching runs (partial page → last page)
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var runs []map[string]any
		switch r.URL.Query().Get("offset") {
		case "", "0":
			runs = page1
		case "100":
			runs = page2
		default:
			t.Errorf("unexpected offset %q", r.URL.Query().Get("offset"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"runs": runs})
	}))
	t.Cleanup(srv.Close)

	baseURL, _ := url.Parse(srv.URL)
	cmd := CountMigRunsCommand{
		Client:  srv.Client(),
		BaseURL: baseURL,
		MigID:   migID,
	}

	count, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 130
	if count != want {
		t.Fatalf("got count=%d, want %d", count, want)
	}
	if callCount != 2 {
		t.Fatalf("got %d page fetches, want 2", callCount)
	}
}
