package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestParseRunIDParam(t *testing.T) {
	t.Parallel()

	validRunID := "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep"

	tests := []struct {
		name      string
		pathKey   string
		pathValue string
		wantID    domaintypes.RunID
		wantErr   bool
		errMsg    string
		wantErrIs error
	}{
		{
			name:      "valid value",
			pathKey:   "id",
			pathValue: validRunID,
			wantID:    domaintypes.RunID(validRunID),
			wantErr:   false,
		},
		{
			name:      "value with leading/trailing whitespace is trimmed",
			pathKey:   "id",
			pathValue: "  " + validRunID + "  ",
			wantID:    domaintypes.RunID(validRunID),
			wantErr:   false,
		},
		{
			name:      "empty value returns error",
			pathKey:   "id",
			pathValue: "",
			wantErr:   true,
			errMsg:    "id path parameter is required",
		},
		{
			name:      "whitespace-only value returns error",
			pathKey:   "id",
			pathValue: "   ",
			wantErr:   true,
			errMsg:    "id path parameter is required",
		},
		{
			name:      "different key name in error message",
			pathKey:   "run_id",
			pathValue: "",
			wantErr:   true,
			errMsg:    "run_id path parameter is required",
		},
		{
			name:      "invalid format returns error",
			pathKey:   "id",
			pathValue: "abc123",
			wantErr:   true,
			wantErrIs: domaintypes.ErrInvalidRunID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.SetPathValue(tt.pathKey, tt.pathValue)

			id, err := ParseRunIDParam(req, tt.pathKey)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("error = %v, want errors.Is(%v)", err, tt.wantErrIs)
					return
				}
				if err.Error() != tt.errMsg {
					if tt.errMsg == "" {
						return
					}
					t.Errorf("error message = %q, want %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if id != tt.wantID {
				t.Errorf("id = %q, want %q", id, tt.wantID)
			}
		})
	}
}

func TestParseJobIDParam(t *testing.T) {
	t.Parallel()

	validJobID := domaintypes.NewJobID().String()

	tests := []struct {
		name      string
		pathKey   string
		pathValue string
		wantID    domaintypes.JobID
		wantErr   bool
		errMsg    string
		wantErrIs error
	}{
		{
			name:      "valid value",
			pathKey:   "job_id",
			pathValue: validJobID,
			wantID:    domaintypes.JobID(validJobID),
			wantErr:   false,
		},
		{
			name:      "empty value returns error",
			pathKey:   "job_id",
			pathValue: "",
			wantErr:   true,
			errMsg:    "job_id path parameter is required",
		},
		{
			name:      "whitespace-only value returns error",
			pathKey:   "job_id",
			pathValue: "   ",
			wantErr:   true,
			errMsg:    "job_id path parameter is required",
		},
		{
			name:      "invalid format returns error",
			pathKey:   "job_id",
			pathValue: "job123",
			wantErr:   true,
			wantErrIs: domaintypes.ErrInvalidJobID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.SetPathValue(tt.pathKey, tt.pathValue)

			id, err := ParseJobIDParam(req, tt.pathKey)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("error = %v, want errors.Is(%v)", err, tt.wantErrIs)
					return
				}
				if err.Error() != tt.errMsg {
					if tt.errMsg == "" {
						return
					}
					t.Errorf("error message = %q, want %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if id != tt.wantID {
				t.Errorf("id = %q, want %q", id, tt.wantID)
			}
		})
	}
}

func TestParseNodeIDParam(t *testing.T) {
	t.Parallel()

	validNodeID := domaintypes.NewNodeKey()

	tests := []struct {
		name      string
		pathKey   string
		pathValue string
		wantID    domaintypes.NodeID
		wantErr   bool
		errMsg    string
		wantErrIs error
	}{
		{
			name:      "valid value",
			pathKey:   "id",
			pathValue: validNodeID,
			wantID:    domaintypes.NodeID(validNodeID),
			wantErr:   false,
		},
		{
			name:      "empty value returns error",
			pathKey:   "id",
			pathValue: "",
			wantErr:   true,
			errMsg:    "id path parameter is required",
		},
		{
			name:      "whitespace-only value returns error",
			pathKey:   "id",
			pathValue: "   ",
			wantErr:   true,
			errMsg:    "id path parameter is required",
		},
		{
			name:      "invalid format returns error",
			pathKey:   "id",
			pathValue: "node123",
			wantErr:   true,
			wantErrIs: domaintypes.ErrInvalidNodeID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.SetPathValue(tt.pathKey, tt.pathValue)

			id, err := ParseNodeIDParam(req, tt.pathKey)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("error = %v, want errors.Is(%v)", err, tt.wantErrIs)
					return
				}
				if err.Error() != tt.errMsg {
					if tt.errMsg == "" {
						return
					}
					t.Errorf("error message = %q, want %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if id != tt.wantID {
				t.Errorf("id = %q, want %q", id, tt.wantID)
			}
		})
	}
}

func TestParseModIDParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pathKey   string
		pathValue string
		wantID    domaintypes.ModID
		wantErr   bool
		errMsg    string
		wantErrIs error
	}{
		{
			name:      "valid value",
			pathKey:   "mod_id",
			pathValue: "mod123",
			wantID:    domaintypes.ModID("mod123"),
			wantErr:   false,
		},
		{
			name:      "empty value returns error",
			pathKey:   "mod_id",
			pathValue: "",
			wantErr:   true,
			errMsg:    "mod_id path parameter is required",
		},
		{
			name:      "whitespace-only value returns error",
			pathKey:   "mod_id",
			pathValue: "   ",
			wantErr:   true,
			errMsg:    "mod_id path parameter is required",
		},
		{
			name:      "invalid format returns error",
			pathKey:   "mod_id",
			pathValue: "mod12",
			wantErr:   true,
			wantErrIs: domaintypes.ErrInvalidModID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.SetPathValue(tt.pathKey, tt.pathValue)

			id, err := ParseModIDParam(req, tt.pathKey)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("error = %v, want errors.Is(%v)", err, tt.wantErrIs)
					return
				}
				if err.Error() != tt.errMsg {
					if tt.errMsg == "" {
						return
					}
					t.Errorf("error message = %q, want %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if id != tt.wantID {
				t.Errorf("id = %q, want %q", id, tt.wantID)
			}
		})
	}
}

func TestParseModRefParam(t *testing.T) {
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
			name:      "empty value returns error",
			pathKey:   "mod_ref",
			pathValue: "",
			wantErr:   true,
			errSubstr: "mod_ref path parameter is required",
		},
		{
			name:      "whitespace-only value returns error",
			pathKey:   "mod_ref",
			pathValue: "   ",
			wantErr:   true,
			errSubstr: "mod_ref path parameter is required",
		},
		{
			name:      "invalid chars (slash) returns error",
			pathKey:   "mod_ref",
			pathValue: "my/mod",
			wantErr:   true,
			errSubstr: "invalid mod ref",
		},
		{
			name:      "invalid chars (question mark) returns error",
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

			ref, err := ParseModRefParam(req, tt.pathKey)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.errSubstr != "" && !strContains(err.Error(), tt.errSubstr) {
					t.Errorf("error message = %q, want to contain %q", err.Error(), tt.errSubstr)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if ref != tt.wantRef {
				t.Errorf("ref = %q, want %q", ref, tt.wantRef)
			}
		})
	}
}

func TestParseModRepoIDParam(t *testing.T) {
	t.Parallel()

	validRepoID := domaintypes.NewModRepoID().String()

	tests := []struct {
		name      string
		pathKey   string
		pathValue string
		wantID    domaintypes.ModRepoID
		wantErr   bool
		errMsg    string
		wantErrIs error
	}{
		{
			name:      "valid value",
			pathKey:   "repo_id",
			pathValue: validRepoID,
			wantID:    domaintypes.ModRepoID(validRepoID),
			wantErr:   false,
		},
		{
			name:      "empty value returns error",
			pathKey:   "repo_id",
			pathValue: "",
			wantErr:   true,
			errMsg:    "repo_id path parameter is required",
		},
		{
			name:      "whitespace-only value returns error",
			pathKey:   "repo_id",
			pathValue: "   ",
			wantErr:   true,
			errMsg:    "repo_id path parameter is required",
		},
		{
			name:      "invalid format returns error",
			pathKey:   "repo_id",
			pathValue: "repo123",
			wantErr:   true,
			wantErrIs: domaintypes.ErrInvalidModRepoID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.SetPathValue(tt.pathKey, tt.pathValue)

			id, err := ParseModRepoIDParam(req, tt.pathKey)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("error = %v, want errors.Is(%v)", err, tt.wantErrIs)
					return
				}
				if err.Error() != tt.errMsg {
					if tt.errMsg == "" {
						return
					}
					t.Errorf("error message = %q, want %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if id != tt.wantID {
				t.Errorf("id = %q, want %q", id, tt.wantID)
			}
		})
	}
}

func TestOptionalRunIDParam(t *testing.T) {
	t.Parallel()

	validRunID := "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep"

	tests := []struct {
		name      string
		pathKey   string
		pathValue string
		wantErr   bool
		wantErrIs error
		wantNil   bool
		wantID    domaintypes.RunID
	}{
		{
			name:      "valid value returns pointer",
			pathKey:   "id",
			pathValue: validRunID,
			wantErr:   false,
			wantNil:   false,
			wantID:    domaintypes.RunID(validRunID),
		},
		{
			name:      "value with whitespace is trimmed",
			pathKey:   "id",
			pathValue: "  " + validRunID + "  ",
			wantErr:   false,
			wantNil:   false,
			wantID:    domaintypes.RunID(validRunID),
		},
		{
			name:      "empty value returns nil",
			pathKey:   "id",
			pathValue: "",
			wantErr:   false,
			wantNil:   true,
		},
		{
			name:      "whitespace-only value returns nil",
			pathKey:   "id",
			pathValue: "   ",
			wantErr:   false,
			wantNil:   true,
		},
		{
			name:      "invalid format returns error",
			pathKey:   "id",
			pathValue: "abc123",
			wantErr:   true,
			wantErrIs: domaintypes.ErrInvalidRunID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.SetPathValue(tt.pathKey, tt.pathValue)

			result, err := OptionalRunIDParam(req, tt.pathKey)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("error = %v, want errors.Is(%v)", err, tt.wantErrIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil but got %q", *result)
				}
				return
			}

			if result == nil {
				t.Errorf("expected non-nil pointer")
				return
			}
			if *result != tt.wantID {
				t.Errorf("id = %q, want %q", *result, tt.wantID)
			}
		})
	}
}

func TestParseRunIDQuery(t *testing.T) {
	t.Parallel()

	validRunID := "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep"

	tests := []struct {
		name      string
		queryKey  string
		query     string
		wantID    domaintypes.RunID
		wantErr   bool
		errMsg    string
		wantErrIs error
	}{
		{
			name:     "valid value",
			queryKey: "id",
			query:    "id=" + validRunID,
			wantID:   domaintypes.RunID(validRunID),
			wantErr:  false,
		},
		{
			name:     "value with whitespace is trimmed",
			queryKey: "id",
			query:    "id=%20" + validRunID + "%20",
			wantID:   domaintypes.RunID(validRunID),
			wantErr:  false,
		},
		{
			name:     "missing query param returns error",
			queryKey: "id",
			query:    "",
			wantErr:  true,
			errMsg:   "id query parameter is required",
		},
		{
			name:     "empty query value returns error",
			queryKey: "id",
			query:    "id=",
			wantErr:  true,
			errMsg:   "id query parameter is required",
		},
		{
			name:      "invalid format returns error",
			queryKey:  "id",
			query:     "id=abc123",
			wantErr:   true,
			wantErrIs: domaintypes.ErrInvalidRunID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			url := "/test"
			if tt.query != "" {
				url = "/test?" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)

			id, err := ParseRunIDQuery(req, tt.queryKey)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Errorf("error = %v, want errors.Is(%v)", err, tt.wantErrIs)
					return
				}
				if err.Error() != tt.errMsg {
					if tt.errMsg == "" {
						return
					}
					t.Errorf("error message = %q, want %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if id != tt.wantID {
				t.Errorf("id = %q, want %q", id, tt.wantID)
			}
		})
	}
}

func TestOptionalRunIDQuery(t *testing.T) {
	t.Parallel()

	validRunID := "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep"

	tests := []struct {
		name      string
		queryKey  string
		query     string
		wantErr   bool
		wantErrIs error
		wantNil   bool
		wantID    domaintypes.RunID
	}{
		{
			name:     "valid value returns pointer",
			queryKey: "id",
			query:    "id=" + validRunID,
			wantErr:  false,
			wantNil:  false,
			wantID:   domaintypes.RunID(validRunID),
		},
		{
			name:     "missing query param returns nil",
			queryKey: "id",
			query:    "",
			wantErr:  false,
			wantNil:  true,
		},
		{
			name:     "empty query value returns nil",
			queryKey: "id",
			query:    "id=",
			wantErr:  false,
			wantNil:  true,
		},
		{
			name:      "invalid format returns error",
			queryKey:  "id",
			query:     "id=abc123",
			wantErr:   true,
			wantErrIs: domaintypes.ErrInvalidRunID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			url := "/test"
			if tt.query != "" {
				url = "/test?" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)

			result, err := OptionalRunIDQuery(req, tt.queryKey)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("error = %v, want errors.Is(%v)", err, tt.wantErrIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil {
				if result != nil {
					t.Errorf("expected nil but got %q", *result)
				}
				return
			}

			if result == nil {
				t.Errorf("expected non-nil pointer")
				return
			}
			if *result != tt.wantID {
				t.Errorf("id = %q, want %q", *result, tt.wantID)
			}
		})
	}
}

// strContains checks if s strContains substr.
func strContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstr(s, substr)))
}

func findSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
