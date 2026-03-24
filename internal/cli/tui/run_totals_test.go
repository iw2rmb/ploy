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

func TestGetRunTotalsCommand(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()

	tests := []struct {
		name          string
		runID         domaintypes.RunID
		runsHandler   http.HandlerFunc
		jobsHandler   http.HandlerFunc
		wantErr       bool
		wantRepoTotal int32
		wantJobTotal  int64
	}{
		{
			name:  "success returns repo and job totals",
			runID: runID,
			runsHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":         runID.String(),
					"status":     "Finished",
					"mig_id":     migID.String(),
					"spec_id":    specID.String(),
					"created_at": time.Now().Format(time.RFC3339),
					"repo_counts": map[string]any{
						"total":          int32(3),
						"queued":         int32(0),
						"running":        int32(0),
						"success":        int32(3),
						"fail":           int32(0),
						"cancelled":      int32(0),
						"derived_status": "completed",
					},
				})
			},
			jobsHandler: func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("run_id"); got != runID.String() {
					t.Errorf("run_id=%q, want %q", got, runID.String())
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"jobs": []any{}, "total": int64(12)})
			},
			wantRepoTotal: 3,
			wantJobTotal:  12,
		},
		{
			name:  "success with no repo counts returns zero repo total",
			runID: runID,
			runsHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":         runID.String(),
					"status":     "Started",
					"mig_id":     migID.String(),
					"spec_id":    specID.String(),
					"created_at": time.Now().Format(time.RFC3339),
				})
			},
			jobsHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"jobs": []any{}, "total": int64(0)})
			},
			wantRepoTotal: 0,
			wantJobTotal:  0,
		},
		{
			name:  "run fetch http error returns error",
			runID: runID,
			runsHandler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			},
			jobsHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"jobs": []any{}, "total": int64(0)})
			},
			wantErr: true,
		},
		{
			name:    "zero run id returns error without hitting server",
			runID:   domaintypes.RunID(""),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mux := http.NewServeMux()
			if tc.runsHandler != nil {
				mux.HandleFunc("/v1/runs/"+runID.String(), tc.runsHandler)
			}
			if tc.jobsHandler != nil {
				mux.HandleFunc("/v1/jobs", tc.jobsHandler)
			}

			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)

			baseURL, _ := url.Parse(srv.URL)
			cmd := GetRunTotalsCommand{
				Client:  srv.Client(),
				BaseURL: baseURL,
				RunID:   tc.runID,
			}

			totals, err := cmd.Run(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got err=%v", tc.wantErr, err)
			}
			if !tc.wantErr {
				if totals.RepoTotal != tc.wantRepoTotal {
					t.Fatalf("RepoTotal=%d, want %d", totals.RepoTotal, tc.wantRepoTotal)
				}
				if totals.JobTotal != tc.wantJobTotal {
					t.Fatalf("JobTotal=%d, want %d", totals.JobTotal, tc.wantJobTotal)
				}
			}
		})
	}
}

func TestGetRunTotalsCommand_NilClient_ReturnsError(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	base, _ := url.Parse("http://localhost")
	cmd := GetRunTotalsCommand{Client: nil, BaseURL: base, RunID: runID}
	_, err := cmd.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}
