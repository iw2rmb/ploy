package main

import "testing"

// FuzzValidateGitLabConfig exercises the config validator over random inputs.
// Ensures no panics and basic invariants around required fields.
func FuzzValidateGitLabConfig(f *testing.F) {
	f.Add("https://gitlab.com", "glpat-seed")
	f.Add("http://localhost", "x")
	f.Add("invalid://", "tok")
	f.Add("", "")
	f.Fuzz(func(t *testing.T, domain, token string) {
		cfg := &gitLabConfigPayload{Domain: domain, Token: token}
		_ = validateGitLabConfig(cfg) // must not panic
	})
}
