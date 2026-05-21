package nodeagent

import (
	"encoding/json"
	"testing"
)

func TestParseSpec_GitLabConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		extra        string
		wantMRFlag   bool
		wantMROnSucc bool
	}{
		{name: "gitlab config from server"},
		{name: "gitlab config with mr flags", extra: `, "mr_on_success": true`, wantMRFlag: true, wantMROnSucc: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			spec := json.RawMessage(`{
				"steps": [{"image": "docker.io/test/mig:latest"}],
				"job_id": "` + testKSUID + `",
				"gitlab_pat": "server-default-token",
				"gitlab_domain": "https://gitlab.example.com"` + tt.extra + `
			}`)

			env, typedOpts, err := parseSpec(spec)
			if err != nil {
				t.Fatalf("parseSpec() error = %v", err)
			}

			if typedOpts.ServerMetadata.JobID.String() != testKSUID {
				t.Errorf("job_id = %q, want %s", typedOpts.ServerMetadata.JobID.String(), testKSUID)
			}
			if typedOpts.MRWiring.GitLabPAT != "server-default-token" {
				t.Errorf("gitlab_pat = %q, want server-default-token", typedOpts.MRWiring.GitLabPAT)
			}
			if typedOpts.MRWiring.GitLabDomain != "https://gitlab.example.com" {
				t.Errorf("gitlab_domain = %q, want https://gitlab.example.com", typedOpts.MRWiring.GitLabDomain)
			}
			if typedOpts.MRFlagsPresent.MROnSuccessSet != tt.wantMRFlag {
				t.Errorf("mr_on_success presence = %v, want %v", typedOpts.MRFlagsPresent.MROnSuccessSet, tt.wantMRFlag)
			}
			if typedOpts.MRWiring.MROnSuccess != tt.wantMROnSucc {
				t.Errorf("mr_on_success = %v, want %v", typedOpts.MRWiring.MROnSuccess, tt.wantMROnSucc)
			}
			for _, key := range []string{"gitlab_pat", "gitlab_domain"} {
				if _, ok := env[key]; ok {
					t.Errorf("%s should not be in env map", key)
				}
			}
		})
	}
}
