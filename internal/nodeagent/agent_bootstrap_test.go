package nodeagent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRequestCertificate_Success verifies successful certificate acquisition on first attempt.
func TestRequestCertificate_Success(t *testing.T) {
	t.Parallel()

	// Create test server that returns a valid certificate response.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path.
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/v1/pki/bootstrap" {
			t.Errorf("expected /v1/pki/bootstrap, got %s", r.URL.Path)
		}

		// Verify authorization header.
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("expected Bearer test-token, got %s", auth)
		}

		// Return valid certificate response.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"certificate": "cert-pem-data",
			"ca_bundle":   "ca-pem-data",
		})
	}))
	defer server.Close()

	cfg := newTestConfig(server.URL)

	agent := &Agent{cfg: cfg}
	ctx := context.Background()

	cert, caCert, err := agent.requestCertificate(ctx, "test-token", []byte("csr-pem-data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cert != "cert-pem-data" {
		t.Errorf("expected cert=cert-pem-data, got %s", cert)
	}
	if caCert != "ca-pem-data" {
		t.Errorf("expected caCert=ca-pem-data, got %s", caCert)
	}
}

// TestRequestCertificate_RetryOnNetworkError verifies retry behavior on network errors.
func TestRequestCertificate_RetryOnNetworkError(t *testing.T) {
	t.Parallel()

	attemptCount := 0

	// Create test server that fails initially then succeeds.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			// Return 500 error to trigger retry.
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Succeed on third attempt.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"certificate": "cert-pem-data",
			"ca_bundle":   "ca-pem-data",
		})
	}))
	defer server.Close()

	cfg := newTestConfig(server.URL)

	agent := &Agent{cfg: cfg}
	ctx := context.Background()

	cert, caCert, err := agent.requestCertificate(ctx, "test-token", []byte("csr-pem-data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cert != "cert-pem-data" {
		t.Errorf("expected cert=cert-pem-data, got %s", cert)
	}
	if caCert != "ca-pem-data" {
		t.Errorf("expected caCert=ca-pem-data, got %s", caCert)
	}

	// Verify retries occurred.
	if attemptCount != 3 {
		t.Errorf("expected 3 attempts, got %d", attemptCount)
	}
}

// TestRequestCertificate_RetryExhaustion verifies failure after max retries.
func TestRequestCertificate_RetryExhaustion(t *testing.T) {
	t.Parallel()

	attemptCount := 0

	// Create test server that always fails.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := newTestConfig(server.URL)

	agent := &Agent{cfg: cfg}
	ctx := context.Background()

	_, _, err := agent.requestCertificate(ctx, "test-token", []byte("csr-pem-data"))
	if err == nil {
		t.Fatal("expected error after retry exhaustion")
	}

	// Verify all 5 attempts were made.
	if attemptCount != 5 {
		t.Errorf("expected 5 attempts, got %d", attemptCount)
	}
}

// TestRequestCertificate_BackoffProgression verifies exponential backoff intervals.
func TestRequestCertificate_BackoffProgression(t *testing.T) {
	t.Parallel()

	attemptCount := 0
	attemptTimes := []time.Time{}

	// Create test server that fails initially then succeeds.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		attemptTimes = append(attemptTimes, time.Now())

		if attemptCount < 4 {
			// Fail first 3 attempts to observe backoff progression.
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		// Succeed on fourth attempt.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"certificate": "cert-pem-data",
			"ca_bundle":   "ca-pem-data",
		})
	}))
	defer server.Close()

	cfg := newTestConfig(server.URL)

	agent := &Agent{cfg: cfg}
	ctx := context.Background()
	start := time.Now()

	_, _, err := agent.requestCertificate(ctx, "test-token", []byte("csr-pem-data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	elapsed := time.Since(start)

	// Verify backoff progression with shared policy (1s initial, 2x multiplier, 50% jitter).
	// Expected delays:
	// - After attempt 1: ~0.5-1.5s (1s ± 50%)
	// - After attempt 2: ~1.0-3.0s (2s ± 50%)
	// - After attempt 3: ~2.0-6.0s (4s ± 50%)
	// Total minimum (lower bound): ~3.5s (0.5s + 1.0s + 2.0s)
	// We use a conservative lower bound to account for jitter.
	expectedMinDuration := 3500 * time.Millisecond
	if elapsed < expectedMinDuration {
		t.Errorf("expected backoff duration >= %v, got %v", expectedMinDuration, elapsed)
	}

	// Verify we got the expected number of attempts.
	if attemptCount != 4 {
		t.Errorf("expected 4 attempts, got %d", attemptCount)
	}

	// Verify intervals are increasing (accounting for jitter).
	// We don't check exact values due to jitter, but ensure progression.
	if len(attemptTimes) >= 3 {
		interval1 := attemptTimes[1].Sub(attemptTimes[0])
		interval2 := attemptTimes[2].Sub(attemptTimes[1])

		// Second interval should generally be larger than first (within jitter tolerance).
		// With 50% jitter: first could be 0.5-1.5s, second could be 1.0-3.0s.
		// Minimum second interval (1.0s) should be at least 0.5s (min first interval / 2).
		if interval2 < interval1/2 {
			t.Errorf("expected increasing backoff intervals, got %v then %v", interval1, interval2)
		}
	}
}

// TestRequestCertificate_ContextCancellation verifies context cancellation during retry.
func TestRequestCertificate_ContextCancellation(t *testing.T) {
	t.Parallel()

	// Create test server that always fails.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := newTestConfig(server.URL)

	agent := &Agent{cfg: cfg}

	// Create context with short timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, _, err := agent.requestCertificate(ctx, "test-token", []byte("csr-pem-data"))
	if err == nil {
		t.Fatal("expected context cancellation error")
	}

	// Verify it's a context error.
	if ctx.Err() == nil {
		t.Error("expected context to be cancelled")
	}
}

// TestRequestCertificate_BearerToken verifies bearer token is saved when provided.
func TestRequestCertificate_BearerToken(t *testing.T) {
	// Note: Cannot use t.Parallel() because we use t.Setenv() to override the bearer token path.

	// Create temporary directory for bearer token.
	tempDir := t.TempDir()
	tokenPath := filepath.Join(tempDir, "bearer-token")

	// Override bearerTokenPath via environment variable for this test.
	t.Setenv("PLOY_NODE_BEARER_TOKEN_PATH", tokenPath)

	// Create test server that returns a bearer token.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"certificate":  "cert-pem-data",
			"ca_bundle":    "ca-pem-data",
			"bearer_token": "secret-bearer-token",
		})
	}))
	defer server.Close()

	cfg := newTestConfig(server.URL)

	agent := &Agent{cfg: cfg}
	ctx := context.Background()

	_, _, err := agent.requestCertificate(ctx, "test-token", []byte("csr-pem-data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify bearer token was saved.
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("failed to read bearer token: %v", err)
	}

	if string(data) != "secret-bearer-token" {
		t.Errorf("expected bearer token=secret-bearer-token, got %s", string(data))
	}
}

// TestRequestCertificate_Non200Status verifies retry on non-200 responses.
func TestRequestCertificate_Non200Status(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		statusCodes  []int // sequence of status codes to return
		wantErr      bool
		wantAttempts int
		wantSucceed  bool
	}{
		{
			name:         "eventual success after 503",
			statusCodes:  []int{http.StatusServiceUnavailable, http.StatusOK},
			wantErr:      false,
			wantAttempts: 2,
			wantSucceed:  true,
		},
		{
			name:         "eventual success after multiple 5xx",
			statusCodes:  []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusOK},
			wantErr:      false,
			wantAttempts: 3,
			wantSucceed:  true,
		},
		{
			name:         "all retries exhausted with 500",
			statusCodes:  []int{http.StatusInternalServerError, http.StatusInternalServerError, http.StatusInternalServerError, http.StatusInternalServerError, http.StatusInternalServerError},
			wantErr:      true,
			wantAttempts: 5, // all 5 attempts
			wantSucceed:  false,
		},
		{
			name:         "4xx errors trigger retry (unlike status uploader)",
			statusCodes:  []int{http.StatusBadRequest, http.StatusOK},
			wantErr:      false,
			wantAttempts: 2,
			wantSucceed:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			attemptCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if attemptCount < len(tt.statusCodes) {
					statusCode := tt.statusCodes[attemptCount]
					attemptCount++

					if statusCode == http.StatusOK {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(statusCode)
						_ = json.NewEncoder(w).Encode(map[string]string{
							"certificate": "cert-pem-data",
							"ca_bundle":   "ca-pem-data",
						})
					} else {
						w.WriteHeader(statusCode)
					}
				} else {
					attemptCount++
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer server.Close()

			cfg := newTestConfig(server.URL)

			agent := &Agent{cfg: cfg}
			ctx := context.Background()

			cert, caCert, err := agent.requestCertificate(ctx, "test-token", []byte("csr-pem-data"))

			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if tt.wantSucceed {
				if cert != "cert-pem-data" {
					t.Errorf("expected cert=cert-pem-data, got %s", cert)
				}
				if caCert != "ca-pem-data" {
					t.Errorf("expected caCert=ca-pem-data, got %s", caCert)
				}
			}

			if attemptCount != tt.wantAttempts {
				t.Errorf("expected %d attempts, got %d", tt.wantAttempts, attemptCount)
			}
		})
	}
}
