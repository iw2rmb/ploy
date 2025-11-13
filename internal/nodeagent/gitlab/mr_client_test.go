package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCreateMR(t *testing.T) {
	tests := []struct {
		name       string
		req        MRCreateRequest
		serverFunc func(w http.ResponseWriter, r *http.Request)
		wantURL    string
		wantErr    bool
	}{
		{
			name: "success",
			req: MRCreateRequest{
				Domain:       "gitlab.example.com",
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-test-token",
				Title:        "Test MR",
				SourceBranch: "feature-branch",
				TargetBranch: "main",
				Description:  "Test description",
				Labels:       "ploy,test",
			},
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				// Verify method.
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}

				// Verify path (URL encoding is preserved in URL.Path by http server).
				expectedPath := "/api/v4/projects/org%2Fproject/merge_requests"
				actualPath := r.URL.EscapedPath()
				if actualPath != expectedPath {
					t.Errorf("expected path %s, got %s", expectedPath, actualPath)
				}

				// Verify Authorization and PRIVATE-TOKEN headers.
				auth := r.Header.Get("Authorization")
				if auth != "Bearer glpat-test-token" {
					t.Errorf("expected Bearer token, got %s", auth)
				}
				priv := r.Header.Get("PRIVATE-TOKEN")
				if priv != "glpat-test-token" {
					t.Errorf("expected PRIVATE-TOKEN header, got %q", priv)
				}

				// Verify Content-Type.
				contentType := r.Header.Get("Content-Type")
				if contentType != "application/json" {
					t.Errorf("expected application/json, got %s", contentType)
				}

				// Decode and verify payload.
				var payload map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("failed to decode payload: %v", err)
				}

				if payload["title"] != "Test MR" {
					t.Errorf("expected title 'Test MR', got %v", payload["title"])
				}
				if payload["source_branch"] != "feature-branch" {
					t.Errorf("expected source_branch 'feature-branch', got %v", payload["source_branch"])
				}
				if payload["target_branch"] != "main" {
					t.Errorf("expected target_branch 'main', got %v", payload["target_branch"])
				}
				if payload["description"] != "Test description" {
					t.Errorf("expected description 'Test description', got %v", payload["description"])
				}
				if payload["labels"] != "ploy,test" {
					t.Errorf("expected labels 'ploy,test', got %v", payload["labels"])
				}

				// Return success response with test server URL.
				w.WriteHeader(http.StatusCreated)
				// Use a mock web_url that doesn't rely on the test server.
				response := map[string]interface{}{
					"iid":     123,
					"web_url": "http://test-server/org/project/-/merge_requests/123",
				}
				_ = json.NewEncoder(w).Encode(response)
			},
			wantURL: "http://test-server/org/project/-/merge_requests/123",
			wantErr: false,
		},
		{
			name: "success_minimal",
			req: MRCreateRequest{
				Domain:       "gitlab.com",
				ProjectID:    "test%2Frepo",
				PAT:          "glpat-minimal",
				Title:        "Minimal MR",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				// Verify minimal payload (no description or labels).
				var payload map[string]interface{}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("failed to decode payload: %v", err)
				}

				if _, ok := payload["description"]; ok {
					t.Errorf("expected no description field, but got one")
				}
				if _, ok := payload["labels"]; ok {
					t.Errorf("expected no labels field, but got one")
				}

				w.WriteHeader(http.StatusCreated)
				response := map[string]interface{}{
					"iid":     456,
					"web_url": "http://test-server/test/repo/-/merge_requests/456",
				}
				_ = json.NewEncoder(w).Encode(response)
			},
			wantURL: "http://test-server/test/repo/-/merge_requests/456",
			wantErr: false,
		},
		{
			name: "gitlab_api_error",
			req: MRCreateRequest{
				Domain:       "gitlab.example.com",
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-bad-token",
				Title:        "Test MR",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message":"401 Unauthorized"}`))
			},
			wantURL: "",
			wantErr: true,
		},
		{
			name: "missing_web_url",
			req: MRCreateRequest{
				Domain:       "gitlab.example.com",
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-token",
				Title:        "Test MR",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			serverFunc: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
				response := map[string]interface{}{
					"iid": 789,
					// Missing web_url field.
				}
				_ = json.NewEncoder(w).Encode(response)
			},
			wantURL: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server.
			server := httptest.NewServer(http.HandlerFunc(tt.serverFunc))
			defer server.Close()

			// Update domain to point to test server.
			// Extract host from server URL (remove http://).
			serverHost := strings.TrimPrefix(server.URL, "http://")
			tt.req.Domain = serverHost

			// Create client.
			client := NewMRClient()

			// Call CreateMR.
			ctx := context.Background()
			gotURL, err := client.CreateMR(ctx, tt.req)

			// Check error.
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateMR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check URL - the web_url returned from the mock server is used as-is.
			if !tt.wantErr {
				if gotURL != tt.wantURL {
					t.Errorf("CreateMR() url = %v, want %v", gotURL, tt.wantURL)
				}
			}
		})
	}
}

func TestCreateMR_Validation(t *testing.T) {
	tests := []struct {
		name    string
		req     MRCreateRequest
		wantErr string
	}{
		{
			name: "missing_domain",
			req: MRCreateRequest{
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-token",
				Title:        "Test",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			wantErr: "domain is required",
		},
		{
			name: "missing_project_id",
			req: MRCreateRequest{
				Domain:       "gitlab.com",
				PAT:          "glpat-token",
				Title:        "Test",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			wantErr: "project_id is required",
		},
		{
			name: "missing_pat",
			req: MRCreateRequest{
				Domain:       "gitlab.com",
				ProjectID:    "org%2Fproject",
				Title:        "Test",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			wantErr: "pat is required",
		},
		{
			name: "missing_title",
			req: MRCreateRequest{
				Domain:       "gitlab.com",
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-token",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			wantErr: "title is required",
		},
		{
			name: "missing_source_branch",
			req: MRCreateRequest{
				Domain:       "gitlab.com",
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-token",
				Title:        "Test",
				TargetBranch: "main",
			},
			wantErr: "source_branch is required",
		},
		{
			name: "missing_target_branch",
			req: MRCreateRequest{
				Domain:       "gitlab.com",
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-token",
				Title:        "Test",
				SourceBranch: "feature",
			},
			wantErr: "target_branch is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewMRClient()
			ctx := context.Background()
			_, err := client.CreateMR(ctx, tt.req)

			if err == nil {
				t.Errorf("expected error, got nil")
				return
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

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

func TestCreateMR_ValidationRedaction(t *testing.T) {
	// Test that validation errors don't leak PAT.
	client := NewMRClient()
	ctx := context.Background()

	req := MRCreateRequest{
		Domain:       "gitlab.com",
		ProjectID:    "test/project",
		PAT:          "glpat-secret-validation-token",
		Title:        "", // Missing title to trigger validation error.
		SourceBranch: "feature",
		TargetBranch: "main",
	}

	_, err := client.CreateMR(ctx, req)

	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	errMsg := err.Error()

	// Verify that the PAT is not in the error message.
	if strings.Contains(errMsg, "glpat-secret-validation-token") {
		t.Errorf("PAT leaked in validation error: %s", errMsg)
	}
}

func TestCreateMR_Retries(t *testing.T) {
	tests := []struct {
		name            string
		req             MRCreateRequest
		serverResponses []struct {
			statusCode int
			body       string
		}
		wantURL      string
		wantErr      bool
		wantAttempts int
	}{
		{
			name: "retry_on_429_then_success",
			req: MRCreateRequest{
				Domain:       "gitlab.example.com",
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-retry-token",
				Title:        "Test MR",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			serverResponses: []struct {
				statusCode int
				body       string
			}{
				{statusCode: http.StatusTooManyRequests, body: `{"message":"Rate limit exceeded"}`},
				{statusCode: http.StatusCreated, body: `{"iid":123,"web_url":"http://test-server/org/project/-/merge_requests/123"}`},
			},
			wantURL:      "http://test-server/org/project/-/merge_requests/123",
			wantErr:      false,
			wantAttempts: 2,
		},
		{
			name: "retry_on_500_then_success",
			req: MRCreateRequest{
				Domain:       "gitlab.example.com",
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-retry-token",
				Title:        "Test MR",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			serverResponses: []struct {
				statusCode int
				body       string
			}{
				{statusCode: http.StatusInternalServerError, body: `{"message":"Internal server error"}`},
				{statusCode: http.StatusCreated, body: `{"iid":456,"web_url":"http://test-server/org/project/-/merge_requests/456"}`},
			},
			wantURL:      "http://test-server/org/project/-/merge_requests/456",
			wantErr:      false,
			wantAttempts: 2,
		},
		{
			name: "retry_on_502_then_success",
			req: MRCreateRequest{
				Domain:       "gitlab.example.com",
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-retry-token",
				Title:        "Test MR",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			serverResponses: []struct {
				statusCode int
				body       string
			}{
				{statusCode: http.StatusBadGateway, body: `{"message":"Bad gateway"}`},
				{statusCode: http.StatusCreated, body: `{"iid":789,"web_url":"http://test-server/org/project/-/merge_requests/789"}`},
			},
			wantURL:      "http://test-server/org/project/-/merge_requests/789",
			wantErr:      false,
			wantAttempts: 2,
		},
		{
			name: "retry_exhausted_429",
			req: MRCreateRequest{
				Domain:       "gitlab.example.com",
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-retry-token",
				Title:        "Test MR",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			serverResponses: []struct {
				statusCode int
				body       string
			}{
				{statusCode: http.StatusTooManyRequests, body: `{"message":"Rate limit exceeded"}`},
				{statusCode: http.StatusTooManyRequests, body: `{"message":"Rate limit exceeded"}`},
				{statusCode: http.StatusTooManyRequests, body: `{"message":"Rate limit exceeded"}`},
			},
			wantURL:      "",
			wantErr:      true,
			wantAttempts: 3,
		},
		{
			name: "retry_exhausted_500",
			req: MRCreateRequest{
				Domain:       "gitlab.example.com",
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-retry-token",
				Title:        "Test MR",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			serverResponses: []struct {
				statusCode int
				body       string
			}{
				{statusCode: http.StatusInternalServerError, body: `{"message":"Internal server error"}`},
				{statusCode: http.StatusServiceUnavailable, body: `{"message":"Service unavailable"}`},
				{statusCode: http.StatusGatewayTimeout, body: `{"message":"Gateway timeout"}`},
			},
			wantURL:      "",
			wantErr:      true,
			wantAttempts: 3,
		},
		{
			name: "no_retry_on_401",
			req: MRCreateRequest{
				Domain:       "gitlab.example.com",
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-retry-token",
				Title:        "Test MR",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			serverResponses: []struct {
				statusCode int
				body       string
			}{
				{statusCode: http.StatusUnauthorized, body: `{"message":"401 Unauthorized"}`},
			},
			wantURL:      "",
			wantErr:      true,
			wantAttempts: 1,
		},
		{
			name: "no_retry_on_400",
			req: MRCreateRequest{
				Domain:       "gitlab.example.com",
				ProjectID:    "org%2Fproject",
				PAT:          "glpat-retry-token",
				Title:        "Test MR",
				SourceBranch: "feature",
				TargetBranch: "main",
			},
			serverResponses: []struct {
				statusCode int
				body       string
			}{
				{statusCode: http.StatusBadRequest, body: `{"message":"Bad request"}`},
			},
			wantURL:      "",
			wantErr:      true,
			wantAttempts: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attemptCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if attemptCount >= len(tt.serverResponses) {
					t.Errorf("unexpected extra request (attempt %d)", attemptCount+1)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				resp := tt.serverResponses[attemptCount]
				attemptCount++
				w.WriteHeader(resp.statusCode)
				_, _ = w.Write([]byte(resp.body))
			}))
			defer server.Close()

			// Update domain to point to test server.
			serverHost := strings.TrimPrefix(server.URL, "http://")
			tt.req.Domain = serverHost

			// Create client.
			client := NewMRClient()

			// Call CreateMR.
			ctx := context.Background()
			gotURL, err := client.CreateMR(ctx, tt.req)

			// Check error.
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateMR() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check URL.
			if !tt.wantErr && gotURL != tt.wantURL {
				t.Errorf("CreateMR() url = %v, want %v", gotURL, tt.wantURL)
			}

			// Check attempt count.
			if attemptCount != tt.wantAttempts {
				t.Errorf("expected %d attempts, got %d", tt.wantAttempts, attemptCount)
			}
		})
	}
}

func TestCreateMR_RetryBackoff(t *testing.T) {
	// Test that backoff increases exponentially.
	attemptCount := 0
	var attemptTimes []time.Time

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptTimes = append(attemptTimes, time.Now())
		attemptCount++
		if attemptCount < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"message":"Rate limit exceeded"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		response := map[string]interface{}{
			"iid":     123,
			"web_url": "http://test-server/org/project/-/merge_requests/123",
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewMRClient()
	ctx := context.Background()

	req := MRCreateRequest{
		Domain:       strings.TrimPrefix(server.URL, "http://"),
		ProjectID:    "org%2Fproject",
		PAT:          "glpat-token",
		Title:        "Test MR",
		SourceBranch: "feature",
		TargetBranch: "main",
	}

	_, err := client.CreateMR(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(attemptTimes) != 3 {
		t.Fatalf("expected 3 attempts, got %d", len(attemptTimes))
	}

	// Check that delays roughly follow exponential backoff (1s, 2s).
	// Allow some tolerance for test execution time (±200ms).
	delay1 := attemptTimes[1].Sub(attemptTimes[0])
	delay2 := attemptTimes[2].Sub(attemptTimes[1])

	const tolerance = 200 * time.Millisecond
	if delay1 < 1*time.Second-tolerance || delay1 > 1*time.Second+tolerance {
		t.Errorf("first retry delay = %v, expected ~1s", delay1)
	}

	if delay2 < 2*time.Second-tolerance || delay2 > 2*time.Second+tolerance {
		t.Errorf("second retry delay = %v, expected ~2s", delay2)
	}
}

func TestCreateMR_ContextCancellation(t *testing.T) {
	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		// Always return 429 to force retries.
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"Rate limit exceeded"}`))
	}))
	defer server.Close()

	client := NewMRClient()

	// Create a context that will be cancelled after first attempt.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Cancel context immediately to prevent retries.
	cancel()

	req := MRCreateRequest{
		Domain:       strings.TrimPrefix(server.URL, "http://"),
		ProjectID:    "org%2Fproject",
		PAT:          "glpat-token",
		Title:        "Test MR",
		SourceBranch: "feature",
		TargetBranch: "main",
	}

	_, err := client.CreateMR(ctx, req)
	if err == nil {
		t.Fatal("expected error due to context cancellation, got nil")
	}

	// Should not retry when context is cancelled.
	if attemptCount > 1 {
		t.Errorf("expected at most 1 attempt with cancelled context, got %d", attemptCount)
	}
}

func TestExtractProjectIDFromURL(t *testing.T) {
	tests := []struct {
		name    string
		repoURL string
		wantID  string
		wantErr bool
	}{
		{
			name:    "standard_https_url",
			repoURL: "https://gitlab.com/org/project.git",
			wantID:  "org%2Fproject",
			wantErr: false,
		},
		{
			name:    "without_git_suffix",
			repoURL: "https://gitlab.com/org/project",
			wantID:  "org%2Fproject",
			wantErr: false,
		},
		{
			name:    "nested_path",
			repoURL: "https://gitlab.example.com/group/subgroup/project.git",
			wantID:  "group%2Fsubgroup%2Fproject",
			wantErr: false,
		},
		{
			name:    "self_hosted",
			repoURL: "https://gitlab.internal.net/team/repo.git",
			wantID:  "team%2Frepo",
			wantErr: false,
		},
		{
			name:    "empty_path",
			repoURL: "https://gitlab.com/",
			wantID:  "",
			wantErr: true,
		},
		{
			name:    "invalid_url",
			repoURL: "not a valid url",
			wantID:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotID, err := ExtractProjectIDFromURL(tt.repoURL)

			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractProjectIDFromURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if gotID != tt.wantID {
				t.Errorf("ExtractProjectIDFromURL() = %v, want %v", gotID, tt.wantID)
			}
		})
	}
}
