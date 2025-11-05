package nodeagent

import (
	"encoding/json"
	"testing"
)

// TestParseSpec_GitLabConfigFromServer verifies that parseSpec correctly extracts
// gitlab_pat and gitlab_domain from spec when supplied by the server.
func TestParseSpec_GitLabConfigFromServer(t *testing.T) {
	t.Parallel()

	spec := json.RawMessage(`{
		"stage_id": "stage-123",
		"gitlab_pat": "server-default-token",
		"gitlab_domain": "https://gitlab.example.com"
	}`)

	opts, env := parseSpec(spec)

	if opts["stage_id"] != "stage-123" {
		t.Errorf("stage_id = %v, want stage-123", opts["stage_id"])
	}

	// Verify gitlab_pat is extracted into opts.
	if opts["gitlab_pat"] != "server-default-token" {
		t.Errorf("gitlab_pat = %v, want server-default-token", opts["gitlab_pat"])
	}
	if opts["gitlab_domain"] != "https://gitlab.example.com" {
		t.Errorf("gitlab_domain = %v, want https://gitlab.example.com", opts["gitlab_domain"])
	}

	// Verify that env is not populated with gitlab config (these are options, not env).
	if _, hasToken := env["gitlab_pat"]; hasToken {
		t.Error("gitlab_pat should not be in env map")
	}
	if _, hasDomain := env["gitlab_domain"]; hasDomain {
		t.Error("gitlab_domain should not be in env map")
	}
}

// TestParseSpec_GitLabConfigWithMRFlags verifies that parseSpec correctly extracts
// gitlab config fields along with MR creation flags.
func TestParseSpec_GitLabConfigWithMRFlags(t *testing.T) {
	t.Parallel()

	spec := json.RawMessage(`{
		"stage_id": "stage-123",
		"gitlab_pat": "server-default-token",
		"gitlab_domain": "https://gitlab.example.com",
		"mr_on_success": true
	}`)

	// Parse the spec.
	opts, _ := parseSpec(spec)

	// Verify all fields are extracted into opts.
	if opts["stage_id"] != "stage-123" {
		t.Errorf("stage_id = %v, want stage-123", opts["stage_id"])
	}
	if opts["gitlab_pat"] != "server-default-token" {
		t.Errorf("gitlab_pat = %v, want server-default-token", opts["gitlab_pat"])
	}
	if opts["gitlab_domain"] != "https://gitlab.example.com" {
		t.Errorf("gitlab_domain = %v, want https://gitlab.example.com", opts["gitlab_domain"])
	}
	if opts["mr_on_success"] != true {
		t.Errorf("mr_on_success = %v, want true", opts["mr_on_success"])
	}
}
