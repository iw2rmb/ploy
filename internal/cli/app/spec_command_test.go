package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainapi "github.com/iw2rmb/ploy/internal/domain/api"
)

func TestSpecCobraCommandRouting(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		setup   func(t *testing.T) []string
		wantOut string
	}{
		{name: "spec push help", args: []string{"spec", "push", "--help"}, wantOut: "ploy spec push [<git-folder>] [flags]"},
		{name: "spec ls help", args: []string{"spec", "ls", "--help"}, wantOut: "ploy spec ls [flags]"},
		{
			name: "spec ls routes to handler",
			args: []string{"spec", "ls"},
			setup: func(t *testing.T) []string {
				t.Helper()
				srv := newAppSpecServer(t)
				t.Setenv("PLOY_SERVER_URL", srv.URL)
				return []string{"spec", "ls"}
			},
			wantOut: "upgrade-java",
		},
		{
			name: "spec push routes to handler",
			setup: func(t *testing.T) []string {
				t.Helper()
				srv := newAppSpecServer(t)
				t.Setenv("PLOY_SERVER_URL", srv.URL)
				t.Setenv("PLOY_CONFIG_HOME", t.TempDir())
				repo := initAppSpecRepo(t)
				return []string{"spec", "push", repo}
			},
			wantOut: "updated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.args
			if tt.setup != nil {
				args = tt.setup(t)
			}
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			root := NewRootCmdWithIO(stdout, stderr)
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("Execute(%v) error = %v", args, err)
			}
			output := stdout.String() + stderr.String()
			if !strings.Contains(output, tt.wantOut) {
				t.Fatalf("output = %q, want containing %q", output, tt.wantOut)
			}
		})
	}
}

func newAppSpecServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/specs" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(domainapi.NamedSpecListResponse{Specs: []domainapi.NamedSpecSummary{{
				ID:                "spec001",
				Name:              "upgrade-java",
				Source:            domainapi.NamedSpecSource{Domain: "github.com", Repo: "acme/service"},
				SHA:               "0123456789abcdef0123456789abcdef01234567",
				SourceCommittedAt: time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC),
				CreatedAt:         time.Date(2026, 6, 19, 12, 1, 0, 0, time.UTC),
			}}})
		case r.URL.Path == "/v1/specs" && r.Method == http.MethodPost:
			var req domainapi.PublishNamedSpecRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(domainapi.NamedSpecSummary{
				ID:                "spec001",
				Name:              req.Name,
				Source:            req.Source,
				SHA:               req.SHA,
				SourceCommittedAt: req.SourceCommittedAt,
				CreatedAt:         req.SourceCommittedAt.Add(time.Minute),
			})
		default:
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func initAppSpecRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runAppGit(t, repo, "init")
	runAppGit(t, repo, "config", "user.email", "spec@example.test")
	runAppGit(t, repo, "config", "user.name", "Spec Tester")
	writeAppFile(t, filepath.Join(repo, "mig.yaml"), `apiVersion: ploy.mig/v1alpha1
name: upgrade-java
steps:
  - image: docker.io/test/mig:latest
`)
	runAppGit(t, repo, "add", ".")
	runAppGit(t, repo, "commit", "-m", "initial")
	runAppGit(t, repo, "remote", "add", "origin", "https://github.com/acme/service.git")
	return repo
}

func runAppGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func writeAppFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
