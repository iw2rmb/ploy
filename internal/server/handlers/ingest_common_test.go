package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
)

func TestParseParam(t *testing.T) {
	t.Parallel()

	t.Run("RunID", func(t *testing.T) {
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
			},
			{
				name:      "value with leading/trailing whitespace is trimmed",
				pathKey:   "id",
				pathValue: "  " + validRunID + "  ",
				wantID:    domaintypes.RunID(validRunID),
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

				id, err := parseParam[domaintypes.RunID](req, tt.pathKey)
				assertIDResult(t, id, err, tt.wantID, tt.wantErr, tt.errMsg, tt.wantErrIs)
			})
		}
	})

	t.Run("JobID", func(t *testing.T) {
		t.Parallel()
		validJobID := domaintypes.NewJobID().String()

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.SetPathValue("job_id", validJobID)
		id, err := parseParam[domaintypes.JobID](req, "job_id")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != domaintypes.JobID(validJobID) {
			t.Errorf("id = %q, want %q", id, validJobID)
		}

		req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req2.SetPathValue("job_id", "")
		_, err = parseParam[domaintypes.JobID](req2, "job_id")
		if err == nil {
			t.Error("expected error for empty value")
		}

		req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req3.SetPathValue("job_id", "job123")
		_, err = parseParam[domaintypes.JobID](req3, "job_id")
		if err == nil || !errors.Is(err, domaintypes.ErrInvalidJobID) {
			t.Errorf("expected ErrInvalidJobID, got %v", err)
		}
	})

	t.Run("NodeID", func(t *testing.T) {
		t.Parallel()
		validNodeID := domaintypes.NewNodeKey()

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.SetPathValue("id", validNodeID)
		id, err := parseParam[domaintypes.NodeID](req, "id")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != domaintypes.NodeID(validNodeID) {
			t.Errorf("id = %q, want %q", id, validNodeID)
		}
	})

	t.Run("MigID", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.SetPathValue("mig_id", "mod123")
		id, err := parseParam[domaintypes.MigID](req, "mig_id")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != domaintypes.MigID("mod123") {
			t.Errorf("id = %q, want %q", id, "mod123")
		}
	})

	t.Run("MigRef", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.SetPathValue("mig_ref", "my-mod")
		ref, err := parseParam[domaintypes.MigRef](req, "mig_ref")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ref != domaintypes.MigRef("my-mod") {
			t.Errorf("ref = %q, want %q", ref, "my-mod")
		}

		// Invalid chars
		req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req2.SetPathValue("mig_ref", "my/mod")
		_, err = parseParam[domaintypes.MigRef](req2, "mig_ref")
		if err == nil {
			t.Error("expected error for invalid mod ref")
		}
	})

	t.Run("MigRepoID", func(t *testing.T) {
		t.Parallel()
		validRepoID := domaintypes.NewMigRepoID().String()

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.SetPathValue("repo_id", validRepoID)
		id, err := parseParam[domaintypes.MigRepoID](req, "repo_id")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != domaintypes.MigRepoID(validRepoID) {
			t.Errorf("id = %q, want %q", id, validRepoID)
		}
	})
}

func TestOptionalParam(t *testing.T) {
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
			wantID:    domaintypes.RunID(validRunID),
		},
		{
			name:      "value with whitespace is trimmed",
			pathKey:   "id",
			pathValue: "  " + validRunID + "  ",
			wantID:    domaintypes.RunID(validRunID),
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

			result, err := optionalParam[domaintypes.RunID](req, tt.pathKey)
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

func TestParseQuery(t *testing.T) {
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
		},
		{
			name:     "value with whitespace is trimmed",
			queryKey: "id",
			query:    "id=%20" + validRunID + "%20",
			wantID:   domaintypes.RunID(validRunID),
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

			id, err := parseQuery[domaintypes.RunID](req, tt.queryKey)
			assertIDResult(t, id, err, tt.wantID, tt.wantErr, tt.errMsg, tt.wantErrIs)
		})
	}
}

func TestOptionalQuery(t *testing.T) {
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
			wantID:   domaintypes.RunID(validRunID),
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

			result, err := optionalQuery[domaintypes.RunID](req, tt.queryKey)
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

// assertIDResult is a test helper for parseParam/parseQuery assertions.
func assertIDResult[T comparable](t *testing.T, got T, err error, want T, wantErr bool, errMsg string, wantErrIs error) {
	t.Helper()
	if wantErr {
		if err == nil {
			t.Errorf("expected error but got nil")
			return
		}
		if wantErrIs != nil && !errors.Is(err, wantErrIs) {
			t.Errorf("error = %v, want errors.Is(%v)", err, wantErrIs)
			return
		}
		if errMsg != "" && err.Error() != errMsg {
			t.Errorf("error message = %q, want %q", err.Error(), errMsg)
		}
		return
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}
	if got != want {
		t.Errorf("got = %v, want %v", got, want)
	}
}
