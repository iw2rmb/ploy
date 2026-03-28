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

// TestRequestCertificate_StatusHandling tests requestCertificate across various
// HTTP status code sequences: success, retry-then-success, and exhaustion.
func TestRequestCertificate_StatusHandling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		statusCodes  []int // sequence of status codes to return
		wantErr      bool
		wantAttempts int
		wantSucceed  bool
		verifyAuth   bool // also verify Authorization header
	}{
		{
			name:         "success on first attempt",
			statusCodes:  []int{http.StatusOK},
			wantErr:      false,
			wantAttempts: 1,
			wantSucceed:  true,
			verifyAuth:   true,
		},
		{
			name:         "retry on 500 then success",
			statusCodes:  []int{http.StatusInternalServerError, http.StatusInternalServerError, http.StatusOK},
			wantErr:      false,
			wantAttempts: 3,
			wantSucceed:  true,
		},
		{
			name:         "all 5 retries exhausted",
			statusCodes:  []int{http.StatusInternalServerError, http.StatusInternalServerError, http.StatusInternalServerError, http.StatusInternalServerError, http.StatusInternalServerError},
			wantErr:      true,
			wantAttempts: 5,
			wantSucceed:  false,
		},
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
			wantAttempts: 5,
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
				if tt.verifyAuth {
					if r.Method != http.MethodPost {
						t.Errorf("expected POST, got %s", r.Method)
					}
					if r.URL.Path != "/v1/pki/bootstrap" {
						t.Errorf("expected /v1/pki/bootstrap, got %s", r.URL.Path)
					}
					auth := r.Header.Get("Authorization")
					if auth != "Bearer test-token" {
						t.Errorf("expected Bearer test-token, got %s", auth)
					}
				}

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

			agent := &Agent{cfg: newTestConfig(server.URL)}

			cert, caCert, err := agent.requestCertificate(context.Background(), "test-token", []byte("csr-pem-data"))

			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if tt.wantSucceed {
				if cert != "cert-pem-data" {
					t.Errorf("cert=%q, want cert-pem-data", cert)
				}
				if caCert != "ca-pem-data" {
					t.Errorf("caCert=%q, want ca-pem-data", caCert)
				}
			}

			if attemptCount != tt.wantAttempts {
				t.Errorf("attempts=%d, want %d", attemptCount, tt.wantAttempts)
			}
		})
	}
}

func TestRequestCertificate_BackoffProgression(t *testing.T) {
	t.Parallel()

	attemptCount := 0
	attemptTimes := []time.Time{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		attemptTimes = append(attemptTimes, time.Now())

		if attemptCount < 4 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"certificate": "cert-pem-data",
			"ca_bundle":   "ca-pem-data",
		})
	}))
	defer server.Close()

	agent := &Agent{cfg: newTestConfig(server.URL)}
	start := time.Now()

	_, _, err := agent.requestCertificate(context.Background(), "test-token", []byte("csr-pem-data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	elapsed := time.Since(start)

	// Expected backoff: ~1s + ~2s + ~4s = ~3.5s minimum (with 50% jitter).
	if elapsed < 3500*time.Millisecond {
		t.Errorf("expected backoff duration >= 3.5s, got %v", elapsed)
	}

	if attemptCount != 4 {
		t.Errorf("attempts=%d, want 4", attemptCount)
	}

	// Verify intervals are increasing (accounting for jitter).
	if len(attemptTimes) >= 3 {
		interval1 := attemptTimes[1].Sub(attemptTimes[0])
		interval2 := attemptTimes[2].Sub(attemptTimes[1])
		if interval2 < interval1/2 {
			t.Errorf("expected increasing backoff, got %v then %v", interval1, interval2)
		}
	}
}

func TestRequestCertificate_ContextCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	agent := &Agent{cfg: newTestConfig(server.URL)}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, _, err := agent.requestCertificate(ctx, "test-token", []byte("csr-pem-data"))
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if ctx.Err() == nil {
		t.Error("expected context to be cancelled")
	}
}

func TestRequestCertificate_BearerToken(t *testing.T) {
	tempDir := t.TempDir()
	tokenPath := filepath.Join(tempDir, "bearer-token")
	t.Setenv("PLOY_NODE_BEARER_TOKEN_PATH", tokenPath)

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

	agent := &Agent{cfg: newTestConfig(server.URL)}

	_, _, err := agent.requestCertificate(context.Background(), "test-token", []byte("csr-pem-data"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("failed to read bearer token: %v", err)
	}
	if string(data) != "secret-bearer-token" {
		t.Errorf("bearer token=%q, want secret-bearer-token", string(data))
	}
}
