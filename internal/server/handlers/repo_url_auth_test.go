package handlers

import "testing"

func TestRepoURLWithGitLabPAT(t *testing.T) {
	t.Run("injects oauth2 token for matching host", func(t *testing.T) {
		got := repoURLWithGitLabPAT(
			"https://gitlab.example.com/group/repo.git",
			"https://gitlab.example.com",
			"glpat-test",
		)
		want := "https://oauth2:glpat-test@gitlab.example.com/group/repo.git"
		if got != want {
			t.Fatalf("repoURLWithGitLabPAT()=%q, want %q", got, want)
		}
	})

	t.Run("does not inject for non-matching host", func(t *testing.T) {
		input := "https://github.com/org/repo.git"
		got := repoURLWithGitLabPAT(input, "gitlab.example.com", "glpat-test")
		if got != input {
			t.Fatalf("repoURLWithGitLabPAT()=%q, want %q", got, input)
		}
	})

	t.Run("does not override existing credentials", func(t *testing.T) {
		input := "https://user:pass@gitlab.example.com/group/repo.git"
		got := repoURLWithGitLabPAT(input, "gitlab.example.com", "glpat-test")
		if got != input {
			t.Fatalf("repoURLWithGitLabPAT()=%q, want %q", got, input)
		}
	})
}
