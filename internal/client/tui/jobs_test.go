package tui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestListJobsCommand(t *testing.T) {
	t.Parallel()

	jobID := domaintypes.NewJobID()
	runID := domaintypes.NewRunID()
	repoID := domaintypes.NewRepoID()

	tests := []struct {
		name      string
		handler   http.HandlerFunc
		limit     int32
		offset    int32
		runID     *domaintypes.RunID
		wantErr   bool
		wantLen   int
		wantTotal int64
	}{
		{
			name: "success returns jobs list with total",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jobs": []map[string]any{
						{
							"job_id":      jobID.String(),
							"name":        "mig-step",
							"status":      "Running",
							"duration_ms": 1200,
							"job_image":   "ghcr.io/iw2rmb/migs-java17:latest",
							"node_id":     "abc123",
							"mig_name":    "java17-upgrade",
							"run_id":      runID.String(),
							"repo_id":     repoID.String(),
						},
					},
					"total": 1,
				})
			},
			wantLen:   1,
			wantTotal: 1,
		},
		{
			name: "success empty list",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"jobs": []any{}, "total": 0})
			},
			wantLen:   0,
			wantTotal: 0,
		},
		{
			name: "sends run_id filter query param",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("run_id"); got != runID.String() {
					t.Errorf("run_id=%q, want %q", got, runID.String())
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"jobs": []any{}, "total": int64(42)})
			},
			runID:     &runID,
			wantLen:   0,
			wantTotal: 42,
		},
		{
			name: "sends limit and offset query params",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("limit"); got != "5" {
					t.Errorf("limit=%q, want %q", got, "5")
				}
				if got := r.URL.Query().Get("offset"); got != "10" {
					t.Errorf("offset=%q, want %q", got, "10")
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"jobs": []any{}, "total": int64(0)})
			},
			limit:  5,
			offset: 10,
		},
		{
			name: "http error returns error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(tc.handler)
			t.Cleanup(srv.Close)

			baseURL, _ := url.Parse(srv.URL)
			cmd := ListJobsCommand{
				Client:  srv.Client(),
				BaseURL: baseURL,
				Limit:   tc.limit,
				Offset:  tc.offset,
				RunID:   tc.runID,
			}

			result, err := cmd.Run(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got err=%v", tc.wantErr, err)
			}
			if !tc.wantErr {
				if len(result.Jobs) != tc.wantLen {
					t.Fatalf("got %d jobs, want %d", len(result.Jobs), tc.wantLen)
				}
				if result.Total != tc.wantTotal {
					t.Fatalf("got total=%d, want %d", result.Total, tc.wantTotal)
				}
				if tc.wantLen == 1 {
					job := result.Jobs[0]
					if got, want := job.Status, domaintypes.JobStatusRunning; got != want {
						t.Fatalf("job status=%q, want %q", got, want)
					}
					if got, want := job.DurationMs, int64(1200); got != want {
						t.Fatalf("job duration=%d, want %d", got, want)
					}
					if got, want := job.JobImage, "ghcr.io/iw2rmb/migs-java17:latest"; got != want {
						t.Fatalf("job image=%q, want %q", got, want)
					}
					if job.NodeID == nil || job.NodeID.String() != "abc123" {
						t.Fatalf("job node_id=%v, want %q", job.NodeID, "abc123")
					}
				}
			}
		})
	}
}

func TestListJobsCommand_NilClient_ReturnsError(t *testing.T) {
	t.Parallel()

	base, _ := url.Parse("http://localhost")
	cmd := ListJobsCommand{Client: nil, BaseURL: base}
	_, err := cmd.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}
