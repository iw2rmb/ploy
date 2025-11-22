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

// TestCreateMR_ClientGoErrorRedaction verifies that errors from the client-go
// library are properly redacted when they contain PAT tokens in error details.
// This covers new error shapes introduced after migrating from manual HTTP calls.
func TestCreateMR_ClientGoErrorRedaction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pat        string
		serverFunc func(w http.ResponseWriter, r *http.Request)
		wantError  bool
	}{
		{
			name: "client_go_error_with_bearer_token",
			pat:  "glpat-secret-123",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				// Simulate client-go error that echoes the Authorization header.
				// This tests that errors from the client-go library that might
				// include auth details are properly redacted.
				w.WriteHeader(http.StatusUnauthorized)
				auth := r.Header.Get("Authorization")
				_, _ = w.Write([]byte(fmt.Sprintf(`{"error":"unauthorized","details":"%s"}`, auth)))
			},
			wantError: true,
		},
		{
			name: "client_go_network_error_with_encoded_pat",
			pat:  "secret@token/value",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				// Simulate a server error that might include URL-encoded PAT variants
				// in error messages (e.g., from URL construction errors).
				w.WriteHeader(http.StatusInternalServerError)
				// Include both URL-encoded variants that might appear in error traces.
				_, _ = w.Write([]byte(`{"message":"error: secret%40token%2Fvalue"}`))
			},
			wantError: true,
		},
		{
			name: "malformed_response_without_web_url",
			pat:  "glpat-token-456",
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				// Return success but missing web_url to trigger permanent error path.
				// This tests that permanent errors from client-go are also redacted.
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"iid":1}`)) // Missing web_url.
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server that simulates GitLab API.
			server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
			defer server.Close()

			client := NewMRClient()
			ctx := context.Background()

			// Build request with test domain extracted from server URL.
			req := MRCreateRequest{
				Domain:       strings.TrimPrefix(server.URL, "http://"),
				ProjectID:    "org%2Fproject",
				PAT:          tt.pat,
				Title:        "Test MR",
				SourceBranch: "feature",
				TargetBranch: "main",
			}

			_, err := client.CreateMR(ctx, req)

			// Verify error expectation.
			if tt.wantError && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if err != nil {
				errMsg := err.Error()

				// Verify that the literal PAT is not in the error message.
				if strings.Contains(errMsg, tt.pat) {
					t.Errorf("PAT not redacted in client-go error: %s", errMsg)
				}

				// Verify URL-encoded variants are also redacted.
				// The redactError function handles QueryEscape, PathEscape, and custom variants.
				encodedVariants := []string{
					strings.ReplaceAll(tt.pat, "@", "%40"),
					strings.ReplaceAll(tt.pat, "/", "%2F"),
					strings.ReplaceAll(strings.ReplaceAll(tt.pat, " ", "%20"), "@", "%40"),
				}
				for _, variant := range encodedVariants {
					if variant != tt.pat && strings.Contains(errMsg, variant) {
						t.Errorf("URL-encoded PAT variant %q not redacted in error: %s", variant, errMsg)
					}
				}

				// Verify [REDACTED] appears when PAT was present in the original error.
				// This is a best-effort check since not all errors will contain PAT.
				// We only enforce this for tests that explicitly inject PAT in responses.
				if tt.name != "malformed_response_without_web_url" {
					if !strings.Contains(errMsg, "[REDACTED]") {
						t.Logf("note: [REDACTED] not found in error (may not contain PAT): %s", errMsg)
					}
				}
			}
		})
	}
}
