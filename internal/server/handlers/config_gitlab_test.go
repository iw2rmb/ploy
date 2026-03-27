package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/config"
)

// TestConfigGitLabGetReturnsCurrentConfig verifies that GET /v1/config/gitlab
// returns the current GitLab configuration stored in the holder.
func TestConfigGitLabGetReturnsCurrentConfig(t *testing.T) {
	holder := NewConfigHolder(config.GitLabConfig{
		Domain: "https://gitlab.example.com",
		Token:  "test-token-123",
	}, nil)

	handler := getGitLabConfigHandler(holder)
	req := httptest.NewRequest(http.MethodGet, "/v1/config/gitlab", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)

	var resp struct {
		Domain string `json:"domain"`
		Token  string `json:"token"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Domain != "https://gitlab.example.com" {
		t.Errorf("Domain = %q, want %q", resp.Domain, "https://gitlab.example.com")
	}
	if resp.Token != "test-token-123" {
		t.Errorf("Token = %q, want %q", resp.Token, "test-token-123")
	}
}

// TestConfigGitLabPutUpdatesConfig verifies that PUT /v1/config/gitlab
// updates the GitLab configuration and returns the new values.
func TestConfigGitLabPutUpdatesConfig(t *testing.T) {
	holder := NewConfigHolder(config.GitLabConfig{
		Domain: "https://gitlab.example.com",
		Token:  "old-token",
	}, nil)

	reqBody := map[string]string{
		"domain": "https://gitlab.new.com",
		"token":  "new-token-456",
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	handler := putGitLabConfigHandler(holder)
	req := httptest.NewRequest(http.MethodPut, "/v1/config/gitlab", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusOK)

	var resp struct {
		Domain string `json:"domain"`
		Token  string `json:"token"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Domain != "https://gitlab.new.com" {
		t.Errorf("response Domain = %q, want %q", resp.Domain, "https://gitlab.new.com")
	}
	if resp.Token != "new-token-456" {
		t.Errorf("response Token = %q, want %q", resp.Token, "new-token-456")
	}

	// Verify the holder was updated.
	cfg := holder.GetGitLab()
	if cfg.Domain != "https://gitlab.new.com" {
		t.Errorf("holder Domain = %q, want %q", cfg.Domain, "https://gitlab.new.com")
	}
	if cfg.Token != "new-token-456" {
		t.Errorf("holder Token = %q, want %q", cfg.Token, "new-token-456")
	}
}

// TestConfigGitLabPutInvalidJSON verifies that PUT /v1/config/gitlab
// returns 400 Bad Request when the request body is not valid JSON.
func TestConfigGitLabPutInvalidJSON(t *testing.T) {
	holder := NewConfigHolder(config.GitLabConfig{}, nil)

	handler := putGitLabConfigHandler(holder)
	req := httptest.NewRequest(http.MethodPut, "/v1/config/gitlab", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
}

// TestConfigGitLabRoundTrip verifies that PUT followed by GET returns the same values.
func TestConfigGitLabRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		token  string
	}{
		{
			name:   "standard values",
			domain: "https://gitlab.com",
			token:  "glpat-abc123",
		},
		{
			name:   "empty values",
			domain: "",
			token:  "",
		},
		{
			name:   "custom domain",
			domain: "https://git.internal.corp",
			token:  "custom-token-xyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			holder := NewConfigHolder(config.GitLabConfig{}, nil)

			// PUT the configuration.
			reqBody := map[string]string{
				"domain": tt.domain,
				"token":  tt.token,
			}
			body, err := json.Marshal(reqBody)
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}

			putHandler := putGitLabConfigHandler(holder)
			putReq := httptest.NewRequest(http.MethodPut, "/v1/config/gitlab", bytes.NewReader(body))
			putReq.Header.Set("Content-Type", "application/json")
			putRR := httptest.NewRecorder()
			putHandler.ServeHTTP(putRR, putReq)

			if putRR.Code != http.StatusOK {
				t.Fatalf("PUT status = %d, want %d", putRR.Code, http.StatusOK)
			}

			// GET the configuration.
			getHandler := getGitLabConfigHandler(holder)
			getReq := httptest.NewRequest(http.MethodGet, "/v1/config/gitlab", nil)
			getRR := httptest.NewRecorder()
			getHandler.ServeHTTP(getRR, getReq)

			if getRR.Code != http.StatusOK {
				t.Fatalf("GET status = %d, want %d", getRR.Code, http.StatusOK)
			}

			var resp struct {
				Domain string `json:"domain"`
				Token  string `json:"token"`
			}
			if err := json.NewDecoder(getRR.Body).Decode(&resp); err != nil {
				t.Fatalf("decode GET response: %v", err)
			}

			if resp.Domain != tt.domain {
				t.Errorf("Domain = %q, want %q", resp.Domain, tt.domain)
			}
			if resp.Token != tt.token {
				t.Errorf("Token = %q, want %q", resp.Token, tt.token)
			}
		})
	}
}
