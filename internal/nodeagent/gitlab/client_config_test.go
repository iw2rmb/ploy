package gitlab

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestNewClient verifies that NewClient creates a properly configured GitLab client
// with correct base URL construction, authentication headers, and error handling.
func TestNewClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cfg         ClientConfig
		wantErr     bool
		wantScheme  string
		errContains string
	}{
		{
			name: "https_for_gitlab_com",
			cfg: ClientConfig{
				Domain: "gitlab.com",
				PAT:    "glpat-test-token",
			},
			wantErr:    false,
			wantScheme: "https",
		},
		{
			name: "https_for_self_hosted",
			cfg: ClientConfig{
				Domain: "gitlab.example.com",
				PAT:    "glpat-test-token",
			},
			wantErr:    false,
			wantScheme: "https",
		},
		{
			name: "http_for_localhost",
			cfg: ClientConfig{
				Domain: "localhost:8080",
				PAT:    "glpat-test-token",
			},
			wantErr:    false,
			wantScheme: "http",
		},
		{
			name: "http_for_127_0_0_1",
			cfg: ClientConfig{
				Domain: "127.0.0.1:3000",
				PAT:    "glpat-test-token",
			},
			wantErr:    false,
			wantScheme: "http",
		},
		{
			name: "error_on_empty_domain",
			cfg: ClientConfig{
				Domain: "",
				PAT:    "glpat-test-token",
			},
			wantErr:     true,
			errContains: "domain is required",
		},
		{
			name: "error_on_empty_pat",
			cfg: ClientConfig{
				Domain: "gitlab.com",
				PAT:    "",
			},
			wantErr:     true,
			errContains: "pat is required",
		},
		{
			name: "custom_http_client",
			cfg: ClientConfig{
				Domain: "gitlab.com",
				PAT:    "glpat-test-token",
				HTTPClient: &http.Client{
					Timeout: 10 * time.Second,
				},
			},
			wantErr:    false,
			wantScheme: "https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, err := NewClient(tt.cfg)

			// Check error expectations.
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("NewClient() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			// Verify client was created.
			if client == nil {
				t.Fatal("NewClient() returned nil client with no error")
			}

			// Verify base URL scheme matches expected.
			baseURL := client.BaseURL().String()
			if !strings.HasPrefix(baseURL, tt.wantScheme+"://") {
				t.Errorf("NewClient() base URL = %v, want scheme %v", baseURL, tt.wantScheme)
			}

			// Verify base URL contains the domain.
			if !strings.Contains(baseURL, tt.cfg.Domain) {
				t.Errorf("NewClient() base URL = %v, want to contain domain %v", baseURL, tt.cfg.Domain)
			}
		})
	}
}

// TestTokenInjector verifies that the tokenInjector transport correctly adds
// the PRIVATE-TOKEN header to all outgoing requests while preserving the
// Authorization header set by client-go.
func TestTokenInjector(t *testing.T) {
	t.Parallel()

	const testPAT = "glpat-secret-token"

	// Create a test server that captures request headers.
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()

	// Extract domain from test server URL (remove scheme).
	serverDomain := strings.TrimPrefix(server.URL, "http://")

	// Create client configured to use the test server.
	client, err := NewClient(ClientConfig{
		Domain: serverDomain,
		PAT:    testPAT,
	})
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	// Make a request to trigger the tokenInjector.
	// We use the low-level Client() method to make a raw request.
	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/v4/user", nil)
	if err != nil {
		t.Fatalf("NewRequest() failed: %v", err)
	}

	// Execute request through the client's HTTP client (which has tokenInjector).
	resp, err := client.HTTPClient().Do(req)
	if err != nil {
		t.Fatalf("Do() failed: %v", err)
	}
	defer resp.Body.Close()

	// Verify PRIVATE-TOKEN header was injected.
	if got := capturedHeaders.Get("PRIVATE-TOKEN"); got != testPAT {
		t.Errorf("PRIVATE-TOKEN header = %q, want %q", got, testPAT)
	}

	// Note: We can't easily verify the Authorization header here because
	// client-go sets it internally. The important thing is that our
	// tokenInjector doesn't interfere with it.
}

// TestTokenInjectorWithNilBaseTransport verifies that tokenInjector falls back
// to http.DefaultTransport when base is nil.
func TestTokenInjectorWithNilBaseTransport(t *testing.T) {
	t.Parallel()

	// Create a simple test server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify PRIVATE-TOKEN header is present.
		if r.Header.Get("PRIVATE-TOKEN") != "test-token" {
			t.Errorf("PRIVATE-TOKEN header not set correctly")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create tokenInjector with nil base transport.
	ti := &tokenInjector{
		base:  nil, // Explicitly nil to test fallback.
		token: "test-token",
	}

	// Make a request through the tokenInjector.
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest() failed: %v", err)
	}

	resp, err := ti.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("RoundTrip() status = %v, want %v", resp.StatusCode, http.StatusOK)
	}
}

// TestClientBaseURL validates that the client targets the expected base URL
// for various domain configurations.
// Note: client-go automatically appends /api/v4 to the base URL.
func TestClientBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		domain      string
		wantBaseURL string
	}{
		{
			name:        "gitlab_com",
			domain:      "gitlab.com",
			wantBaseURL: "https://gitlab.com/api/v4",
		},
		{
			name:        "self_hosted",
			domain:      "gitlab.internal.net",
			wantBaseURL: "https://gitlab.internal.net/api/v4",
		},
		{
			name:        "localhost_with_port",
			domain:      "localhost:8080",
			wantBaseURL: "http://localhost:8080/api/v4",
		},
		{
			name:        "ip_address",
			domain:      "127.0.0.1:3000",
			wantBaseURL: "http://127.0.0.1:3000/api/v4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client, err := NewClient(ClientConfig{
				Domain: tt.domain,
				PAT:    "glpat-test-token",
			})
			if err != nil {
				t.Fatalf("NewClient() failed: %v", err)
			}

			// Verify base URL matches expected.
			gotBaseURL := client.BaseURL().String()
			// Trim trailing slash if present for comparison.
			gotBaseURL = strings.TrimSuffix(gotBaseURL, "/")

			if gotBaseURL != tt.wantBaseURL {
				t.Errorf("client.BaseURL() = %v, want %v", gotBaseURL, tt.wantBaseURL)
			}
		})
	}
}
