package arf

import (
	"os"
	"strings"
	"testing"
)

func TestAuthenticatedRemoteURL_GitLabTokenInjection(t *testing.T) {
	g := NewGitOperations("")
	orig := "https://gitlab.com/namespace/project.git"

	// No token: URL unchanged
	_ = os.Unsetenv("GITLAB_TOKEN")
	if got := g.authenticatedRemoteURL(orig); got != orig {
		t.Fatalf("expected unchanged URL when no token set; got %q", got)
	}

	// With token: inject oauth2 credentials
	_ = os.Setenv("GITLAB_TOKEN", "test-token-123")
	defer func() { _ = os.Unsetenv("GITLAB_TOKEN") }()
	got := g.authenticatedRemoteURL(orig)
	if !strings.HasPrefix(got, "https://oauth2:test-token-123@") {
		t.Fatalf("expected oauth2 credentials injected; got %q", got)
	}
}
