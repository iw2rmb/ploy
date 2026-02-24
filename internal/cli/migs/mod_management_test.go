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

	"github.com/iw2rmb/ploy/internal/domain/types"
)

// TestAddModCommand_Run validates AddModCommand responses.
func TestAddModCommand_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		modName     string
		spec        *json.RawMessage
		serverResp  AddModResult
		statusCode  int
		wantErr     bool
		wantErrText string
	}{
		{
			name:       "successful create without spec",
			modName:    "test-mod",
			spec:       nil,
			statusCode: http.StatusCreated,
			serverResp: AddModResult{
				ID:        types.MigID("mod001"),
				Name:      "test-mod",
				SpecID:    nil,
				CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name:       "successful create with spec",
			modName:    "test-mod-with-spec",
			spec:       jsonRawPtr([]byte(`{"version":"v1"}`)),
			statusCode: http.StatusCreated,
			serverResp: AddModResult{
				ID:        types.MigID("mod002"),
				Name:      "test-mod-with-spec",
				SpecID:    specIDPtr(types.SpecID("spec-001")),
				CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		},
		{
			name:        "missing name",
			modName:     "",
			wantErr:     true,
			wantErrText: "name is required",
		},
		{
			name:        "missing client",
			modName:     "test-mod",
			wantErr:     true,
			wantErrText: "http client required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify POST method to /v1/migs.
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != "/v1/migs" {
					t.Errorf("expected path /v1/migs, got %s", r.URL.Path)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				_ = json.NewEncoder(w).Encode(tc.serverResp)
			}))
			t.Cleanup(srv.Close)

			baseURL, err := url.Parse(srv.URL)
			if err != nil {
				t.Fatalf("parse server URL: %v", err)
			}

			var client *http.Client
			if !tc.wantErr || !strings.Contains(tc.wantErrText, "http client required") {
				client = srv.Client()
			}

			cmd := AddModCommand{
				Client:  client,
				BaseURL: baseURL,
				Name:    tc.modName,
				Spec:    tc.spec,
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
			if result.ID != tc.serverResp.ID {
				t.Errorf("got ID %q, want %q", result.ID, tc.serverResp.ID)
			}
			if result.Name != tc.serverResp.Name {
				t.Errorf("got Name %q, want %q", result.Name, tc.serverResp.Name)
			}
		})
	}
}

// TestListMigsCommand_Run validates ListMigsCommand responses.
func TestListMigsCommand_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		limit      int32
		offset     int32
		serverResp []ModSummary
		wantCount  int
	}{
		{
			name:   "list mods with results",
			limit:  50,
			offset: 0,
			serverResp: []ModSummary{
				{ID: types.MigID("mod001"), Name: "mod-one", Archived: false, CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				{ID: types.MigID("mod002"), Name: "mod-two", Archived: true, CreatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
			},
			wantCount: 2,
		},
		{
			name:       "empty list",
			limit:      50,
			offset:     0,
			serverResp: []ModSummary{},
			wantCount:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected GET, got %s", r.Method)
				}
				if !strings.HasPrefix(r.URL.Path, "/v1/migs") {
					t.Errorf("expected path /v1/migs, got %s", r.URL.Path)
				}

				resp := struct {
					Mods []ModSummary `json:"migs"`
				}{Mods: tc.serverResp}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			}))
			t.Cleanup(srv.Close)

			baseURL, _ := url.Parse(srv.URL)

			cmd := ListMigsCommand{
				Client:  srv.Client(),
				BaseURL: baseURL,
				Limit:   tc.limit,
				Offset:  tc.offset,
			}

			result, err := cmd.Run(context.Background())
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			if len(result) != tc.wantCount {
				t.Errorf("got %d results, want %d", len(result), tc.wantCount)
			}
		})
	}
}

// TestRemoveModCommand_Run validates RemoveModCommand responses.
func TestRemoveModCommand_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		modID       string
		statusCode  int
		wantErr     bool
		wantErrText string
	}{
		{
			name:       "successful delete",
			modID:      "mod001",
			statusCode: http.StatusNoContent,
		},
		{
			name:        "mod not found",
			modID:       "nonexistent",
			statusCode:  http.StatusNotFound,
			wantErr:     true,
			wantErrText: "not found",
		},
		{
			name:        "mod has runs",
			modID:       "mod-with-runs",
			statusCode:  http.StatusConflict,
			wantErr:     true,
			wantErrText: "existing runs",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete {
					t.Errorf("expected DELETE, got %s", r.Method)
				}
				if !strings.HasPrefix(r.URL.Path, "/v1/migs/") {
					t.Errorf("expected path /v1/migs/{id}, got %s", r.URL.Path)
				}

				if tc.statusCode == http.StatusNoContent {
					w.WriteHeader(http.StatusNoContent)
					return
				}

				w.WriteHeader(tc.statusCode)
				if tc.wantErrText != "" {
					_, _ = w.Write([]byte(tc.wantErrText))
				}
			}))
			t.Cleanup(srv.Close)

			baseURL, _ := url.Parse(srv.URL)

			cmd := RemoveModCommand{
				Client:  srv.Client(),
				BaseURL: baseURL,
				MigRef:  types.MigRef(tc.modID),
			}

			err := cmd.Run(context.Background())
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
		})
	}
}

// TestArchiveMigCommand_Run validates ArchiveMigCommand responses.
func TestArchiveMigCommand_Run(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/archive") {
			t.Errorf("expected path to contain /archive, got %s", r.URL.Path)
		}

		resp := ArchiveMigResult{ID: types.MigID("mod001"), Name: "test-mod", Archived: true}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	baseURL, _ := url.Parse(srv.URL)

	cmd := ArchiveMigCommand{
		Client:  srv.Client(),
		BaseURL: baseURL,
		MigRef:  types.MigRef("mod001"),
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !result.Archived {
		t.Error("expected Archived to be true")
	}
}

// TestUnarchiveMigCommand_Run validates UnarchiveMigCommand responses.
func TestUnarchiveMigCommand_Run(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/unarchive") {
			t.Errorf("expected path to contain /unarchive, got %s", r.URL.Path)
		}

		resp := UnarchiveMigResult{ID: types.MigID("mod001"), Name: "test-mod", Archived: false}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	baseURL, _ := url.Parse(srv.URL)

	cmd := UnarchiveMigCommand{
		Client:  srv.Client(),
		BaseURL: baseURL,
		MigRef:  types.MigRef("mod001"),
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.Archived {
		t.Error("expected Archived to be false")
	}
}

// TestSetModSpecCommand_Run validates SetModSpecCommand responses.
func TestSetModSpecCommand_Run(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/specs") {
			t.Errorf("expected path to contain /specs, got %s", r.URL.Path)
		}

		resp := SetModSpecResult{ID: types.SpecID("spec-001"), CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	baseURL, _ := url.Parse(srv.URL)

	cmd := SetModSpecCommand{
		Client:  srv.Client(),
		BaseURL: baseURL,
		MigRef:  types.MigRef("mod001"),
		Spec:    json.RawMessage(`{"version":"v1"}`),
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if result.ID.String() != "spec-001" {
		t.Errorf("got ID %q, want %q", result.ID.String(), "spec-001")
	}
}

// TestResolveModByNameNoHeuristic verifies that ResolveModByNameCommand does NOT
// special-case "UUID-like" inputs. The command should always query the server for
// name resolution, regardless of input format. This test ensures there are no
// client-side heuristics that bypass server resolution.
func TestResolveModByNameNoHeuristic(t *testing.T) {
	t.Parallel()

	// A UUID-like string that historically was special-cased.
	// The old code would return this as-is without querying the server.
	uuidLike := "12345678-1234-1234-1234-123456789012"

	// Track whether the server was queried.
	serverQueried := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverQueried = true

		// Verify it's a list request with name_substring filter.
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/v1/migs") {
			t.Errorf("expected path /v1/migs, got %s", r.URL.Path)
		}

		// Check that the UUID-like string is passed as a filter.
		nameSubstring := r.URL.Query().Get("name_substring")
		if nameSubstring != uuidLike {
			t.Errorf("expected name_substring=%q, got %q", uuidLike, nameSubstring)
		}

		// Return empty list (no match).
		resp := struct {
			Mods []ModSummary `json:"migs"`
		}{Mods: []ModSummary{}}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	baseURL, _ := url.Parse(srv.URL)

	cmd := ResolveModByNameCommand{
		Client:  srv.Client(),
		BaseURL: baseURL,
		MigRef:  types.MigRef(uuidLike),
	}

	result, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// The key assertion: server MUST have been queried even for UUID-like inputs.
	// This verifies no client-side heuristic bypassed the server.
	if !serverQueried {
		t.Error("server was not queried for UUID-like input; heuristic may still exist")
	}

	// Result should be the original ref (no name match found).
	if result != uuidLike {
		t.Errorf("got result %q, want %q", result, uuidLike)
	}
}

// Helper functions for tests.
func jsonRawPtr(data []byte) *json.RawMessage {
	raw := json.RawMessage(data)
	return &raw
}

func specIDPtr(v types.SpecID) *types.SpecID {
	return &v
}
