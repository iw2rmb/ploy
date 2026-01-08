package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

// TestPathParamsUseDomainTypes validates that handlers correctly use domain type
// helpers to parse path parameters, returning 400 errors for invalid/blank IDs
// before making store calls.
//
// This test ensures the handler boundary validation pattern:
//   - Empty path params → 400 Bad Request
//   - Whitespace-only params → 400 Bad Request
//   - Valid params → typed domain value
func TestPathParamsUseDomainTypes(t *testing.T) {
	t.Parallel()

	t.Run("ParseRunIDParam", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			pathKey   string
			pathValue string
			wantID    domaintypes.RunID
			wantErr   bool
			errSubstr string
		}{
			{
				name:      "valid KSUID",
				pathKey:   "id",
				pathValue: "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
				wantID:    domaintypes.RunID("2HBZ1MRFOo8uvXVJhVqKlf8W8Ep"),
				wantErr:   false,
			},
			{
				name:      "empty returns 400",
				pathKey:   "id",
				pathValue: "",
				wantErr:   true,
				errSubstr: "id path parameter is required",
			},
			{
				name:      "whitespace-only returns 400",
				pathKey:   "id",
				pathValue: "   ",
				wantErr:   true,
				errSubstr: "id path parameter is required",
			},
			{
				name:      "whitespace trimmed",
				pathKey:   "run_id",
				pathValue: "  abc123  ",
				wantID:    domaintypes.RunID("abc123"),
				wantErr:   false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.SetPathValue(tt.pathKey, tt.pathValue)

				id, err := domaintypes.ParseRunIDParam(req, tt.pathKey)

				if tt.wantErr {
					if err == nil {
						t.Fatalf("expected error but got nil")
					}
					if !containsSubstr(err.Error(), tt.errSubstr) {
						t.Errorf("error = %q, want to contain %q", err.Error(), tt.errSubstr)
					}
					return
				}

				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if id != tt.wantID {
					t.Errorf("id = %q, want %q", id, tt.wantID)
				}
			})
		}
	})

	t.Run("ParseJobIDParam", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			pathKey   string
			pathValue string
			wantID    domaintypes.JobID
			wantErr   bool
			errSubstr string
		}{
			{
				name:      "valid KSUID",
				pathKey:   "job_id",
				pathValue: "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
				wantID:    domaintypes.JobID("2HBZ1MRFOo8uvXVJhVqKlf8W8Ep"),
				wantErr:   false,
			},
			{
				name:      "empty returns 400",
				pathKey:   "job_id",
				pathValue: "",
				wantErr:   true,
				errSubstr: "job_id path parameter is required",
			},
			{
				name:      "whitespace-only returns 400",
				pathKey:   "job_id",
				pathValue: "   ",
				wantErr:   true,
				errSubstr: "job_id path parameter is required",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.SetPathValue(tt.pathKey, tt.pathValue)

				id, err := domaintypes.ParseJobIDParam(req, tt.pathKey)

				if tt.wantErr {
					if err == nil {
						t.Fatalf("expected error but got nil")
					}
					if !containsSubstr(err.Error(), tt.errSubstr) {
						t.Errorf("error = %q, want to contain %q", err.Error(), tt.errSubstr)
					}
					return
				}

				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if id != tt.wantID {
					t.Errorf("id = %q, want %q", id, tt.wantID)
				}
			})
		}
	})

	t.Run("ParseNodeIDParam", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			pathKey   string
			pathValue string
			wantID    domaintypes.NodeID
			wantErr   bool
			errSubstr string
		}{
			{
				name:      "valid NanoID",
				pathKey:   "id",
				pathValue: "abc123",
				wantID:    domaintypes.NodeID("abc123"),
				wantErr:   false,
			},
			{
				name:      "empty returns 400",
				pathKey:   "id",
				pathValue: "",
				wantErr:   true,
				errSubstr: "id path parameter is required",
			},
			{
				name:      "whitespace-only returns 400",
				pathKey:   "id",
				pathValue: "   ",
				wantErr:   true,
				errSubstr: "id path parameter is required",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.SetPathValue(tt.pathKey, tt.pathValue)

				id, err := domaintypes.ParseNodeIDParam(req, tt.pathKey)

				if tt.wantErr {
					if err == nil {
						t.Fatalf("expected error but got nil")
					}
					if !containsSubstr(err.Error(), tt.errSubstr) {
						t.Errorf("error = %q, want to contain %q", err.Error(), tt.errSubstr)
					}
					return
				}

				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if id != tt.wantID {
					t.Errorf("id = %q, want %q", id, tt.wantID)
				}
			})
		}
	})

	t.Run("ParseModIDParam", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			pathKey   string
			pathValue string
			wantID    domaintypes.ModID
			wantErr   bool
			errSubstr string
		}{
			{
				name:      "valid NanoID",
				pathKey:   "mod_id",
				pathValue: "abc123",
				wantID:    domaintypes.ModID("abc123"),
				wantErr:   false,
			},
			{
				name:      "empty returns 400",
				pathKey:   "mod_id",
				pathValue: "",
				wantErr:   true,
				errSubstr: "mod_id path parameter is required",
			},
			{
				name:      "whitespace-only returns 400",
				pathKey:   "mod_id",
				pathValue: "   ",
				wantErr:   true,
				errSubstr: "mod_id path parameter is required",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.SetPathValue(tt.pathKey, tt.pathValue)

				id, err := domaintypes.ParseModIDParam(req, tt.pathKey)

				if tt.wantErr {
					if err == nil {
						t.Fatalf("expected error but got nil")
					}
					if !containsSubstr(err.Error(), tt.errSubstr) {
						t.Errorf("error = %q, want to contain %q", err.Error(), tt.errSubstr)
					}
					return
				}

				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if id != tt.wantID {
					t.Errorf("id = %q, want %q", id, tt.wantID)
				}
			})
		}
	})

	t.Run("ParseModRefParam", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			pathKey   string
			pathValue string
			wantRef   domaintypes.ModRef
			wantErr   bool
			errSubstr string
		}{
			{
				name:      "valid mod name",
				pathKey:   "mod_ref",
				pathValue: "my-mod",
				wantRef:   domaintypes.ModRef("my-mod"),
				wantErr:   false,
			},
			{
				name:      "valid mod id",
				pathKey:   "mod_ref",
				pathValue: "abc123",
				wantRef:   domaintypes.ModRef("abc123"),
				wantErr:   false,
			},
			{
				name:      "empty returns 400",
				pathKey:   "mod_ref",
				pathValue: "",
				wantErr:   true,
				errSubstr: "mod_ref path parameter is required",
			},
			{
				name:      "whitespace-only returns 400",
				pathKey:   "mod_ref",
				pathValue: "   ",
				wantErr:   true,
				errSubstr: "mod_ref path parameter is required",
			},
			{
				name:      "invalid chars (slash) returns 400",
				pathKey:   "mod_ref",
				pathValue: "my/mod",
				wantErr:   true,
				errSubstr: "invalid mod ref",
			},
			{
				name:      "invalid chars (question mark) returns 400",
				pathKey:   "mod_ref",
				pathValue: "mod?name",
				wantErr:   true,
				errSubstr: "invalid mod ref",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.SetPathValue(tt.pathKey, tt.pathValue)

				ref, err := domaintypes.ParseModRefParam(req, tt.pathKey)

				if tt.wantErr {
					if err == nil {
						t.Fatalf("expected error but got nil")
					}
					if !containsSubstr(err.Error(), tt.errSubstr) {
						t.Errorf("error = %q, want to contain %q", err.Error(), tt.errSubstr)
					}
					return
				}

				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if ref != tt.wantRef {
					t.Errorf("ref = %q, want %q", ref, tt.wantRef)
				}
			})
		}
	})

	t.Run("ParseModRepoIDParam", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name      string
			pathKey   string
			pathValue string
			wantID    domaintypes.ModRepoID
			wantErr   bool
			errSubstr string
		}{
			{
				name:      "valid NanoID",
				pathKey:   "repo_id",
				pathValue: "abc12345",
				wantID:    domaintypes.ModRepoID("abc12345"),
				wantErr:   false,
			},
			{
				name:      "empty returns 400",
				pathKey:   "repo_id",
				pathValue: "",
				wantErr:   true,
				errSubstr: "repo_id path parameter is required",
			},
			{
				name:      "whitespace-only returns 400",
				pathKey:   "repo_id",
				pathValue: "   ",
				wantErr:   true,
				errSubstr: "repo_id path parameter is required",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				req := httptest.NewRequest(http.MethodGet, "/test", nil)
				req.SetPathValue(tt.pathKey, tt.pathValue)

				id, err := domaintypes.ParseModRepoIDParam(req, tt.pathKey)

				if tt.wantErr {
					if err == nil {
						t.Fatalf("expected error but got nil")
					}
					if !containsSubstr(err.Error(), tt.errSubstr) {
						t.Errorf("error = %q, want to contain %q", err.Error(), tt.errSubstr)
					}
					return
				}

				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if id != tt.wantID {
					t.Errorf("id = %q, want %q", id, tt.wantID)
				}
			})
		}
	})
}

// containsSubstr checks if s contains substr.
func containsSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
