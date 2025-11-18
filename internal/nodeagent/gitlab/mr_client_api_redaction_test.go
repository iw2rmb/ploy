package gitlab

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// PAT redaction tests: ensure secrets are redacted in API and validation errors.

// TestCreateMR_PATRedaction verifies that PAT tokens are redacted from error
// messages, including URL-encoded variants with special chars (@, /, etc).
func TestCreateMR_PATRedaction(t *testing.T) {
	tests := []struct {
		name       string
		pat        string
		serverFunc func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name: "api_error_with_pat_in_response",
			pat:  "glpat-secret-token-12345",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				// Echo the auth header in the error (simulating a real API that might leak tokens).
				auth := r.Header.Get("Authorization")
				_, _ = w.Write([]byte(fmt.Sprintf(`{"message":"Invalid token: %s"}`, auth)))
			},
		},
		{
			name: "pat_with_special_chars",
			pat:  "glpat-token@special",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				auth := r.Header.Get("Authorization")
				_, _ = w.Write([]byte(fmt.Sprintf(`{"message":"Auth failed: %s"}`, auth)))
			},
		},
		{
			name: "url_encoded_pat_in_error",
			pat:  "token@value",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				// Simulate error with URL-encoded PAT.
				_, _ = w.Write([]byte(`{"message":"Invalid: token%40value"}`))
			},
		},
		{
			name: "url_encoded_slash_in_error",
			pat:  "tok/en",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				// Simulate error where PAT slash is percent-encoded.
				_, _ = w.Write([]byte(`{"message":"Invalid: tok%2Fen"}`))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server.
			server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
			defer server.Close()

			client := NewMRClient()
			ctx := context.Background()

			req := MRCreateRequest{
				Domain:       strings.TrimPrefix(server.URL, "http://"),
				ProjectID:    "test%2Fproject",
				PAT:          tt.pat,
				Title:        "Test",
				SourceBranch: "feature",
				TargetBranch: "main",
			}

			_, err := client.CreateMR(ctx, req)

			if err == nil {
				t.Fatal("expected error, got nil")
			}

			errMsg := err.Error()

			// Verify that the secret token is redacted.
			if strings.Contains(errMsg, tt.pat) {
				t.Errorf("PAT not redacted in error message: %s", errMsg)
			}

			// Verify that [REDACTED] appears instead.
			if !strings.Contains(errMsg, "[REDACTED]") {
				t.Errorf("expected [REDACTED] in error message, got: %s", errMsg)
			}
		})
	}
}

// TestCreateMR_ValidationRedaction verifies that validation errors don't leak
// PAT tokens even when they appear in request fields.
func TestCreateMR_ValidationRedaction(t *testing.T) {
	client := NewMRClient()
	ctx := context.Background()

	pat := "glpat-secret-token-12345"
	req := MRCreateRequest{
		Domain:       "gitlab.com",
		ProjectID:    "test/project",
		PAT:          pat,
		Title:        "", // Missing title to trigger validation error.
		SourceBranch: "feature",
		TargetBranch: "main",
	}

	_, err := client.CreateMR(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()

	// Verify that the PAT is not in the error message.
	if strings.Contains(errMsg, pat) {
		t.Errorf("PAT leaked in validation error message: %s", errMsg)
	}
}
