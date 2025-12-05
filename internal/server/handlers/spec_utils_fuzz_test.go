package handlers

import (
	"encoding/json"
	"testing"

	"github.com/iw2rmb/ploy/internal/server/config"
)

// FuzzMergeGitLabConfigIntoSpec_NoPanic ensures the merge helper never panics
// and always returns valid JSON for arbitrary inputs.
func FuzzMergeGitLabConfigIntoSpec_NoPanic(f *testing.F) {
	f.Add([]byte(`{"job_id":"j"}`), "https://gitlab.example.com", "glpat-xxx")
	f.Add([]byte(`{"gitlab_pat":"per","gitlab_domain":"d"}`), "", "token")
	f.Add([]byte(`not json`), "d", "t")

	f.Fuzz(func(t *testing.T, specBytes []byte, domain, token string) {
		spec := json.RawMessage(specBytes)
		cfg := config.GitLabConfig{Domain: domain, Token: token}
		out := mergeGitLabConfigIntoSpec(spec, cfg)
		if !json.Valid(out) {
			t.Fatalf("merge produced invalid JSON: %q", string(out))
		}
	})
}
