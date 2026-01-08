package types

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseRunIDParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pathKey   string
		pathValue string
		wantID    RunID
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid value",
			pathKey:   "id",
			pathValue: "abc123",
			wantID:    RunID("abc123"),
			wantErr:   false,
		},
		{
			name:      "value with leading/trailing whitespace is trimmed",
			pathKey:   "id",
			pathValue: "  abc123  ",
			wantID:    RunID("abc123"),
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
			name:      "KSUID-like value",
			pathKey:   "id",
			pathValue: "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
			wantID:    RunID("2HBZ1MRFOo8uvXVJhVqKlf8W8Ep"),
			wantErr:   false,
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
				if err.Error() != tt.errMsg {
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

	tests := []struct {
		name      string
		pathKey   string
		pathValue string
		wantID    JobID
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid value",
			pathKey:   "job_id",
			pathValue: "job123",
			wantID:    JobID("job123"),
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
				if err.Error() != tt.errMsg {
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

	tests := []struct {
		name      string
		pathKey   string
		pathValue string
		wantID    NodeID
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid value",
			pathKey:   "id",
			pathValue: "node123",
			wantID:    NodeID("node123"),
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
				if err.Error() != tt.errMsg {
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
		wantID    ModID
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid value",
			pathKey:   "mod_id",
			pathValue: "mod123",
			wantID:    ModID("mod123"),
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
				if err.Error() != tt.errMsg {
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
		wantRef   ModRef
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "valid mod name",
			pathKey:   "mod_ref",
			pathValue: "my-mod",
			wantRef:   ModRef("my-mod"),
			wantErr:   false,
		},
		{
			name:      "valid mod id",
			pathKey:   "mod_ref",
			pathValue: "abc123",
			wantRef:   ModRef("abc123"),
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

	tests := []struct {
		name      string
		pathKey   string
		pathValue string
		wantID    ModRepoID
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid value",
			pathKey:   "repo_id",
			pathValue: "repo123",
			wantID:    ModRepoID("repo123"),
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
				if err.Error() != tt.errMsg {
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

	tests := []struct {
		name      string
		pathKey   string
		pathValue string
		wantNil   bool
		wantID    RunID
	}{
		{
			name:      "valid value returns pointer",
			pathKey:   "id",
			pathValue: "abc123",
			wantNil:   false,
			wantID:    RunID("abc123"),
		},
		{
			name:      "value with whitespace is trimmed",
			pathKey:   "id",
			pathValue: "  abc123  ",
			wantNil:   false,
			wantID:    RunID("abc123"),
		},
		{
			name:      "empty value returns nil",
			pathKey:   "id",
			pathValue: "",
			wantNil:   true,
		},
		{
			name:      "whitespace-only value returns nil",
			pathKey:   "id",
			pathValue: "   ",
			wantNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.SetPathValue(tt.pathKey, tt.pathValue)

			result := OptionalRunIDParam(req, tt.pathKey)

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

	tests := []struct {
		name     string
		queryKey string
		query    string
		wantID   RunID
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid value",
			queryKey: "id",
			query:    "id=abc123",
			wantID:   RunID("abc123"),
			wantErr:  false,
		},
		{
			name:     "value with whitespace is trimmed",
			queryKey: "id",
			query:    "id=%20abc123%20",
			wantID:   RunID("abc123"),
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
				if err.Error() != tt.errMsg {
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

	tests := []struct {
		name     string
		queryKey string
		query    string
		wantNil  bool
		wantID   RunID
	}{
		{
			name:     "valid value returns pointer",
			queryKey: "id",
			query:    "id=abc123",
			wantNil:  false,
			wantID:   RunID("abc123"),
		},
		{
			name:     "missing query param returns nil",
			queryKey: "id",
			query:    "",
			wantNil:  true,
		},
		{
			name:     "empty query value returns nil",
			queryKey: "id",
			query:    "id=",
			wantNil:  true,
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

			result := OptionalRunIDQuery(req, tt.queryKey)

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
