package handlers

import (
	"encoding/json"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/config"
)

// FuzzApplyGitLabConfigMutator_NoPanic ensures the mutator never panics
// and always produces valid JSON for arbitrary inputs.
func FuzzApplyGitLabConfigMutator_NoPanic(f *testing.F) {
	f.Add([]byte(`{"job_id":"j"}`), "https://gitlab.example.com", "glpat-xxx")
	f.Add([]byte(`{"gitlab_pat":"per","gitlab_domain":"d"}`), "", "token")
	f.Add([]byte(`not json`), "d", "t")

	f.Fuzz(func(t *testing.T, specBytes []byte, domain, token string) {
		spec := json.RawMessage(specBytes)
		cfg := config.GitLabConfig{Domain: domain, Token: token}

		m, err := parseSpecObjectStrict(spec)
		if err != nil {
			return
		}

		if err := applyGitLabConfigMutator(m, cfg); err != nil {
			return
		}

		out, err := marshalSpecObject(m)
		if err != nil {
			t.Fatalf("marshalSpecObject failed: %v", err)
		}
		if !json.Valid(out) {
			t.Fatalf("produced invalid JSON: %q", string(out))
		}
	})
}
