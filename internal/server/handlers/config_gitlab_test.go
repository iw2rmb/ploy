package handlers

import (
	"net/http"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/config"
)

// TestConfigGitLabRoundTrip verifies that PUT followed by GET returns the same values,
// and that the holder is updated after PUT.
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

			// Verify PUT response.
			putResp := decodeBody[gitLabConfigResponse](t, putRR)
			if putResp.Domain != tt.domain {
				t.Errorf("PUT response Domain = %q, want %q", putResp.Domain, tt.domain)
			}
			if putResp.Token != tt.token {
				t.Errorf("PUT response Token = %q, want %q", putResp.Token, tt.token)
			}

			// Verify holder was updated.
			cfg := holder.GetGitLab()
			if cfg.Domain != tt.domain {
				t.Errorf("holder Domain = %q, want %q", cfg.Domain, tt.domain)
			}
			if cfg.Token != tt.token {
				t.Errorf("holder Token = %q, want %q", cfg.Token, tt.token)
			}

			// GET the configuration.
			getRR := doRequest(t, getGitLabConfigHandler(holder), http.MethodGet, "/v1/config/gitlab", nil)
			assertStatus(t, getRR, http.StatusOK)

			resp := decodeBody[gitLabConfigResponse](t, getRR)
			if resp.Domain != tt.domain {
				t.Errorf("GET Domain = %q, want %q", resp.Domain, tt.domain)
			}
			if resp.Token != tt.token {
				t.Errorf("GET Token = %q, want %q", resp.Token, tt.token)
			}
		})
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
