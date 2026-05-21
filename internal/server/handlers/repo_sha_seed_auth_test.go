package handlers

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/gitauth"
)

func TestGitLSRemoteUsesCleanURLAndAuthEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake git is not portable to windows")
	}

	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}
	argsPath := filepath.Join(dir, "args.txt")
	envPath := filepath.Join(dir, "env.txt")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CAPTURE_ARGS\"\nenv > \"$CAPTURE_ENV\"\necho '0123456789abcdef0123456789abcdef01234567\trefs/heads/main'\n"
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CAPTURE_ARGS", argsPath)
	t.Setenv("CAPTURE_ENV", envPath)

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

	args := readTestFile(t, argsPath)
	if strings.Contains(args, "glpat-secret") {
		t.Fatalf("git args contain PAT: %q", args)
	}
	if !strings.Contains(args, "https://gitlab.example.com/group/repo.git") {
		t.Fatalf("git args do not contain clean URL: %q", args)
	}

	env := readTestFile(t, envPath)
	if !strings.Contains(env, "GIT_CONFIG_KEY_0=http.https://gitlab.example.com/.extraHeader") {
		t.Fatalf("git env missing scoped extraHeader key: %q", env)
	}
	wantHeader := "GIT_CONFIG_VALUE_0=Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("oauth2:glpat-secret"))
	if !strings.Contains(env, wantHeader) {
		t.Fatalf("git env missing auth header")
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
