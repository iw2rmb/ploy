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

func TestListMigsCommand(t *testing.T) {
	t.Parallel()

	migID := domaintypes.NewMigID()

	tests := []struct {
		name       string
		handler    http.HandlerFunc
		limit      int32
		offset     int32
		wantErr    bool
		wantLen    int
		checkQuery func(t *testing.T, r *http.Request)
	}{
		{
			name: "success returns migs list",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"migs": []map[string]any{
						{
							"id":         migID.String(),
							"name":       "java17-upgrade",
							"archived":   false,
							"created_at": "2026-01-10T00:00:00Z",
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
				_ = json.NewEncoder(w).Encode(map[string]any{"migs": []any{}})
			},
			wantLen: 0,
		},
		{
			name: "sends limit and offset query params",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("limit"); got != "10" {
					t.Errorf("limit=%q, want %q", got, "10")
				}
				if got := r.URL.Query().Get("offset"); got != "5" {
					t.Errorf("offset=%q, want %q", got, "5")
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"migs": []any{}})
			},
			limit:   10,
			offset:  5,
			wantLen: 0,
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
			cmd := ListMigsCommand{
				Client:  srv.Client(),
				BaseURL: baseURL,
				Limit:   tc.limit,
				Offset:  tc.offset,
			}

			result, err := cmd.Run(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got err=%v", tc.wantErr, err)
			}
			if !tc.wantErr && len(result.Migs) != tc.wantLen {
				t.Fatalf("got %d migs, want %d", len(result.Migs), tc.wantLen)
			}
		})
	}
}

func TestListMigsCommand_NilClient_ReturnsError(t *testing.T) {
	t.Parallel()

	base, _ := url.Parse("http://localhost")
	cmd := ListMigsCommand{Client: nil, BaseURL: base}
	_, err := cmd.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}
