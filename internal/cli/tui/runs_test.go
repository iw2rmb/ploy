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

func TestListRunsCommand(t *testing.T) {
	t.Parallel()

	runID := domaintypes.NewRunID()
	migID := domaintypes.NewMigID()
	specID := domaintypes.NewSpecID()

	tests := []struct {
		name    string
		handler http.HandlerFunc
		limit   int32
		offset  int32
		wantErr bool
		wantLen int
	}{
		{
			name: "success returns runs list",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"runs": []map[string]any{
						{
							"id":         runID.String(),
							"status":     "Started",
							"mig_id":     migID.String(),
							"spec_id":    specID.String(),
							"created_at": time.Now().Format(time.RFC3339),
						},
					},
				})
			},
			wantLen: 1,
		},
		{
			name: "success empty list",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"runs": []any{}})
			},
			wantLen: 0,
		},
		{
			name: "sends limit and offset query params",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("limit"); got != "20" {
					t.Errorf("limit=%q, want %q", got, "20")
				}
				if got := r.URL.Query().Get("offset"); got != "40" {
					t.Errorf("offset=%q, want %q", got, "40")
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"runs": []any{}})
			},
			limit:   20,
			offset:  40,
			wantLen: 0,
		},
		{
			name: "http error returns error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
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
			cmd := ListRunsCommand{
				Client:  srv.Client(),
				BaseURL: baseURL,
				Limit:   tc.limit,
				Offset:  tc.offset,
			}

			result, err := cmd.Run(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got err=%v", tc.wantErr, err)
			}
			if !tc.wantErr && len(result.Runs) != tc.wantLen {
				t.Fatalf("got %d runs, want %d", len(result.Runs), tc.wantLen)
			}
		})
	}
}

func TestListRunsCommand_NilBaseURL_ReturnsError(t *testing.T) {
	t.Parallel()

	cmd := ListRunsCommand{Client: &http.Client{}, BaseURL: nil}
	_, err := cmd.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for nil base url")
	}
}
