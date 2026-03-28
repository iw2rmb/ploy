package handlers

import (
	"net/http"
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
	rr := doRequest(t, handler, http.MethodGet, "/v1/config/gitlab", nil)

	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[gitLabConfigResponse](t, rr)

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

	handler := putGitLabConfigHandler(holder)

	reqBody := map[string]string{
		"domain": "https://gitlab.new.com",
		"token":  "new-token-456",
	}

	rr := doRequest(t, handler, http.MethodPut, "/v1/config/gitlab", reqBody)

	assertStatus(t, rr, http.StatusOK)

	resp := decodeBody[gitLabConfigResponse](t, rr)

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
	rr := doRequest(t, handler, http.MethodPut, "/v1/config/gitlab", "not json")

	assertStatus(t, rr, http.StatusBadRequest)
}

// TestConfigGitLabRoundTrip verifies that PUT followed by GET returns the same values.
func TestConfigGitLabRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		token  string
	}{
		{name: "standard values", domain: "https://gitlab.com", token: "glpat-abc123"},
		{name: "empty values", domain: "", token: ""},
		{name: "custom domain", domain: "https://git.internal.corp", token: "custom-token-xyz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			holder := NewConfigHolder(config.GitLabConfig{}, nil)

			// PUT the configuration.
			putRR := doRequest(t, putGitLabConfigHandler(holder), http.MethodPut, "/v1/config/gitlab", map[string]string{
				"domain": tt.domain,
				"token":  tt.token,
			})
			assertStatus(t, putRR, http.StatusOK)

			// GET the configuration.
			getRR := doRequest(t, getGitLabConfigHandler(holder), http.MethodGet, "/v1/config/gitlab", nil)
			assertStatus(t, getRR, http.StatusOK)

			resp := decodeBody[gitLabConfigResponse](t, getRR)

			if resp.Domain != tt.domain {
				t.Errorf("Domain = %q, want %q", resp.Domain, tt.domain)
			}
			if resp.Token != tt.token {
				t.Errorf("Token = %q, want %q", resp.Token, tt.token)
			}
		})
	}
}
