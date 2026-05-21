package handlers

import "testing"

func TestGitAuthOptionsFromSpec(t *testing.T) {
	spec := []byte(`{
		"steps": [{"image": "ubuntu:latest", "command": "true"}],
		"gitlab_pat": "  glpat-test  ",
		"gitlab_domain": " https://gitlab.example.com "
	}`)

	got := gitAuthOptionsFromSpec(spec)
	if got.GitLabPAT != "glpat-test" {
		t.Fatalf("GitLabPAT=%q, want glpat-test", got.GitLabPAT)
	}
	if got.GitLabDomain != "https://gitlab.example.com" {
		t.Fatalf("GitLabDomain=%q, want https://gitlab.example.com", got.GitLabDomain)
	}
}
