package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRequiredPathParam validates the requiredPathParam helper returns correct
// values and errors for various path parameter scenarios.
func TestRequiredPathParam(t *testing.T) {
	tests := []struct {
		name      string
		pathKey   string
		pathValue string
		wantVal   string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid value",
			pathKey:   "id",
			pathValue: "abc123",
			wantVal:   "abc123",
			wantErr:   false,
		},
		{
			name:      "value with leading/trailing whitespace is trimmed",
			pathKey:   "id",
			pathValue: "  abc123  ",
			wantVal:   "abc123",
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
			pathKey:   "repo_id",
			pathValue: "",
			wantErr:   true,
			errMsg:    "repo_id path parameter is required",
		},
		{
			name:      "KSUID-like value",
			pathKey:   "id",
			pathValue: "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
			wantVal:   "2HBZ1MRFOo8uvXVJhVqKlf8W8Ep",
			wantErr:   false,
		},
		{
			name:      "NanoID-like value",
			pathKey:   "repo_id",
			pathValue: "abc12345",
			wantVal:   "abc12345",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test request with the path value set.
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.SetPathValue(tt.pathKey, tt.pathValue)

			val, err := requiredPathParam(req, tt.pathKey)

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
			if val != tt.wantVal {
				t.Errorf("value = %q, want %q", val, tt.wantVal)
			}
		})
	}
}

// TestOptionalPathParam validates the optionalPathParam helper returns correct
// values for various path parameter scenarios.
func TestOptionalPathParam(t *testing.T) {
	tests := []struct {
		name      string
		pathKey   string
		pathValue string
		wantNil   bool
		wantVal   string
	}{
		{
			name:      "valid value returns pointer",
			pathKey:   "id",
			pathValue: "abc123",
			wantNil:   false,
			wantVal:   "abc123",
		},
		{
			name:      "value with whitespace is trimmed",
			pathKey:   "id",
			pathValue: "  abc123  ",
			wantNil:   false,
			wantVal:   "abc123",
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
			name:      "missing key returns nil",
			pathKey:   "nonexistent",
			pathValue: "",
			wantNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test request with the path value set.
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			if tt.pathValue != "" || tt.pathKey == "id" {
				req.SetPathValue(tt.pathKey, tt.pathValue)
			}

			result := optionalPathParam(req, tt.pathKey)

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
			if *result != tt.wantVal {
				t.Errorf("value = %q, want %q", *result, tt.wantVal)
			}
		})
	}
}

// TestDecodeRejectsUnknownFields validates that DecodeJSON rejects requests
// containing unknown JSON fields, returning 400 Bad Request.
func TestDecodeRejectsUnknownFields(t *testing.T) {
	type testRequest struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantErr    bool // true if DecodeJSON returns error
	}{
		{
			name:       "valid request with known fields",
			body:       `{"name": "test", "value": 42}`,
			wantStatus: http.StatusOK, // no error response
			wantErr:    false,
		},
		{
			name:       "request with unknown field",
			body:       `{"name": "test", "value": 42, "unknown_field": "bad"}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "request with only unknown field",
			body:       `{"unknown_field": "bad"}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "empty object is valid",
			body:       `{}`,
			wantStatus: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "malformed JSON",
			body:       `{"name": "test"`,
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			var dest testRequest
			err := DecodeJSON(rr, req, &dest, DefaultMaxBodySize)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				if rr.Code != tt.wantStatus {
					t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestDecodeRejectsLargeBody validates that DecodeJSON rejects requests
// exceeding the maxBytes limit, returning 413 Request Entity Too Large.
func TestDecodeRejectsLargeBody(t *testing.T) {
	type testRequest struct {
		Data string `json:"data"`
	}

	// Create a body that exceeds 1 KiB limit.
	largeData := strings.Repeat("x", 2048)
	body := `{"data": "` + largeData + `"}`

	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	var dest testRequest
	err := DecodeJSON(rr, req, &dest, 1024) // 1 KiB limit

	if err == nil {
		t.Error("expected error for oversized body")
	}
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusRequestEntityTooLarge)
	}
}

// TestDecodeRequiresExactlyOneJSONValue validates that DecodeJSON accepts exactly
// one JSON value and rejects trailing non-whitespace or additional JSON values.
func TestDecodeRequiresExactlyOneJSONValue(t *testing.T) {
	type testRequest struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantErr    bool
	}{
		{
			name:       "single json value is valid",
			body:       `{"name":"test","value":42}`,
			wantStatus: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "single json value with trailing whitespace is valid",
			body:       "{\"name\":\"test\",\"value\":42}\n\t  ",
			wantStatus: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "trailing non-whitespace is rejected",
			body:       `{"name":"test","value":42} trailing`,
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
		{
			name:       "multiple json values are rejected",
			body:       `{"name":"test","value":42} {"name":"next","value":7}`,
			wantStatus: http.StatusBadRequest,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			var dest testRequest
			err := DecodeJSON(rr, req, &dest, DefaultMaxBodySize)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if rr.Code != tt.wantStatus {
					t.Fatalf("status = %d, want %d", rr.Code, tt.wantStatus)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
