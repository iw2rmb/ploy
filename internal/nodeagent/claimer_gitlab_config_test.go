package nodeagent

import (
	"encoding/json"
	"testing"
)

func TestParseSpec_GitLabConfigFromServer(t *testing.T) {
	t.Parallel()

	spec := json.RawMessage(`{
		"steps": [{"image": "docker.io/test/mig:latest"}],
		"job_id": "` + testKSUID + `",
		"gitlab_pat": "server-default-token",
		"gitlab_domain": "https://gitlab.example.com"
	}`)

	env, typedOpts, _ := parseSpec(spec)

	if typedOpts.ServerMetadata.JobID.String() != testKSUID {
		t.Errorf("job_id = %q, want %s", typedOpts.ServerMetadata.JobID.String(), testKSUID)
	}

	// Verify gitlab config is extracted into typed options.
	if typedOpts.MRWiring.GitLabPAT != "server-default-token" {
		t.Errorf("gitlab_pat = %q, want server-default-token", typedOpts.MRWiring.GitLabPAT)
	}
	if typedOpts.MRWiring.GitLabDomain != "https://gitlab.example.com" {
		t.Errorf("gitlab_domain = %q, want https://gitlab.example.com", typedOpts.MRWiring.GitLabDomain)
	}

	// Verify that env is not populated with gitlab config (these are options, not env).
	if _, hasToken := env["gitlab_pat"]; hasToken {
		t.Error("gitlab_pat should not be in env map")
	}
	if _, hasDomain := env["gitlab_domain"]; hasDomain {
		t.Error("gitlab_domain should not be in env map")
	}
}

func TestParseSpec_GitLabConfigWithMRFlags(t *testing.T) {
	t.Parallel()

	spec := json.RawMessage(`{
		"steps": [{"image": "docker.io/test/mig:latest"}],
		"job_id": "` + testKSUID + `",
		"gitlab_pat": "server-default-token",
		"gitlab_domain": "https://gitlab.example.com",
		"mr_on_success": true
	}`)

	// Parse the spec.
	_, typedOpts, _ := parseSpec(spec)

	// Verify all fields are extracted into typed options.
	if typedOpts.ServerMetadata.JobID.String() != testKSUID {
		t.Errorf("job_id = %q, want %s", typedOpts.ServerMetadata.JobID.String(), testKSUID)
	}
	if typedOpts.MRWiring.GitLabPAT != "server-default-token" {
		t.Errorf("gitlab_pat = %q, want server-default-token", typedOpts.MRWiring.GitLabPAT)
	}
	if typedOpts.MRWiring.GitLabDomain != "https://gitlab.example.com" {
		t.Errorf("gitlab_domain = %q, want https://gitlab.example.com", typedOpts.MRWiring.GitLabDomain)
	}
	if !typedOpts.MRFlagsPresent.MROnSuccessSet {
		t.Errorf("mr_on_success presence = false, want true")
	}
	if typedOpts.MRWiring.MROnSuccess != true {
		t.Errorf("mr_on_success = %v, want true", typedOpts.MRWiring.MROnSuccess)
	}
}
