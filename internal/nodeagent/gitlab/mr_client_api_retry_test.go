package gitlab

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Retry behavior tests: retry policy, backoff, and context cancellation.

// TestCreateMR_RetryBehavior verifies retry behavior for 429/5xx responses and
// that no retry occurs for 4xx errors like 400/401.
func TestCreateMR_RetryBehavior(t *testing.T) {
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

// TestCreateMR_RetryBackoff verifies that retry delays follow exponential
// backoff (1s, 2s) with reasonable tolerance for test execution timing.
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
	// The shared backoff helper uses 50% jitter (randomization factor),
	// so delays can vary: 1s ± 50% = [0.5s, 1.5s], 2s ± 50% = [1s, 3s].
	// Allow wider tolerance to account for jitter and test execution time.
	delay1 := attemptTimes[1].Sub(attemptTimes[0])
	delay2 := attemptTimes[2].Sub(attemptTimes[1])

	// First delay: 1s ± 50% jitter = [0.5s, 1.5s].
	// Add 200ms tolerance for test execution overhead.
	const minDelay1 = 500*time.Millisecond - 200*time.Millisecond
	const maxDelay1 = 1500*time.Millisecond + 200*time.Millisecond
	if delay1 < minDelay1 || delay1 > maxDelay1 {
		t.Errorf("first retry delay = %v, expected ~1s with 50%% jitter [%v, %v]", delay1, minDelay1, maxDelay1)
	}

	// Second delay: 2s ± 50% jitter = [1s, 3s].
	// Add 200ms tolerance for test execution overhead.
	const minDelay2 = 1000*time.Millisecond - 200*time.Millisecond
	const maxDelay2 = 3000*time.Millisecond + 200*time.Millisecond
	if delay2 < minDelay2 || delay2 > maxDelay2 {
		t.Errorf("second retry delay = %v, expected ~2s with 50%% jitter [%v, %v]", delay2, minDelay2, maxDelay2)
	}
}

// TestCreateMR_ContextCancellation verifies that CreateMR respects context
// cancellation and doesn't retry when the context is already cancelled.
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

	// Create a context that will be cancelled before retries.
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
