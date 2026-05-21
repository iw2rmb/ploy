package handlers

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/gitauth"
	"github.com/iw2rmb/ploy/internal/testutil/fakegit"
)

func TestGitLSRemoteUsesCleanURLAndAuthEnv(t *testing.T) {
	capture := fakegit.Install(t, "0123456789abcdef0123456789abcdef01234567\trefs/heads/main")

	sha, err := gitLSRemote(
		context.Background(),
		"https://oauth2:glpat-secret@gitlab.example.com/group/repo.git",
		"refs/heads/main",
		gitauth.Options{},
	)
	if err != nil {
		t.Fatalf("gitLSRemote() error: %v", err)
	}
	if sha != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("sha=%q", sha)
	}

	args := fakegit.Read(t, capture.ArgsPath)
	if strings.Contains(args, "glpat-secret") {
		t.Fatalf("git args contain PAT: %q", args)
	}
	if !strings.Contains(args, "https://gitlab.example.com/group/repo.git") {
		t.Fatalf("git args do not contain clean URL: %q", args)
	}

	env := fakegit.Read(t, capture.EnvPath)
	if !strings.Contains(env, "GIT_CONFIG_KEY_0=http.https://gitlab.example.com/.extraHeader") {
		t.Fatalf("git env missing scoped extraHeader key: %q", env)
	}
	wantHeader := "GIT_CONFIG_VALUE_0=Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("oauth2:glpat-secret"))
	if !strings.Contains(env, wantHeader) {
		t.Fatalf("git env missing auth header")
	}
}
