package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cfg "github.com/iw2rmb/ploy/internal/config/gitlab"
)

func TestConfigGitlabShowPrintsSanitizedConfig(t *testing.T) {
	t.Helper()
	originalFactory := gitlabConfigStoreFactory
	t.Cleanup(func() { gitlabConfigStoreFactory = originalFactory })

	stub := &stubGitlabStore{
		config: cfg.Config{
			APIBaseURL:      "https://gitlab.example.com",
			AllowedProjects: []string{"group/project"},
			DefaultToken: cfg.Token{
				Name:   "primary",
				Value:  "secret",
				Scopes: []string{"api", "read_repository"},
			},
			RBAC: cfg.RBAC{Readers: []string{"ops"}, Updaters: []string{"secops"}},
		},
		revision: 7,
	}
	gitlabConfigStoreFactory = func(context.Context) (gitlabStore, error) {
		return stub, nil
	}

	buf := &bytes.Buffer{}
	if err := execute([]string{"config", "gitlab", "show"}, buf); err != nil {
		t.Fatalf("execute show: %v", err)
	}
	output := buf.String()
	if strings.Contains(output, "secret") {
		t.Fatalf("expected token redacted in output, got %q", output)
	}
	if !strings.Contains(output, "https://gitlab.example.com") {
		t.Fatalf("expected base URL in output, got %q", output)
	}
	if !strings.Contains(output, "revision 7") {
		t.Fatalf("expected revision in output, got %q", output)
	}
}

func TestConfigGitlabSetLoadsFileAndPersists(t *testing.T) {
	t.Helper()
	originalFactory := gitlabConfigStoreFactory
	t.Cleanup(func() { gitlabConfigStoreFactory = originalFactory })

	stub := &stubGitlabStore{}
	gitlabConfigStoreFactory = func(context.Context) (gitlabStore, error) {
		return stub, nil
	}

	file := filepath.Join(t.TempDir(), "config.json")
	payload := cfg.Config{
		APIBaseURL:      "https://gitlab.example.com",
		AllowedProjects: []string{"group/project"},
		DefaultToken: cfg.Token{
			Name:   "primary",
			Value:  "secret",
			Scopes: []string{"api", "read_repository"},
		},
		RBAC: cfg.RBAC{Readers: []string{"ops"}, Updaters: []string{"secops"}},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	buf := &bytes.Buffer{}
	if err := execute([]string{"config", "gitlab", "set", "--file", file}, buf); err != nil {
		t.Fatalf("execute set: %v", err)
	}

	if !stub.saved {
		t.Fatalf("expected store Save to be called")
	}
	if stub.savedConfig.APIBaseURL != "https://gitlab.example.com" {
		t.Fatalf("unexpected APIBaseURL saved: %+v", stub.savedConfig)
	}
	if stub.savedConfig.DefaultToken.Value != "secret" {
		t.Fatalf("expected secret stored, got %q", stub.savedConfig.DefaultToken.Value)
	}
	if !strings.Contains(buf.String(), "GitLab configuration updated") {
		t.Fatalf("expected success message, got %q", buf.String())
	}
}

type stubGitlabStore struct {
	config      cfg.Config
	revision    int64
	loadErr     error
	saveErr     error
	saved       bool
	savedConfig cfg.Config
}

func (s *stubGitlabStore) Load(context.Context) (cfg.Config, int64, error) {
	return s.config, s.revision, s.loadErr
}

func (s *stubGitlabStore) Save(_ context.Context, config cfg.Config) (int64, error) {
	s.saved = true
	s.savedConfig = config
	if s.saveErr != nil {
		return 0, s.saveErr
	}
	if s.revision == 0 {
		s.revision = 1
	} else {
		s.revision++
	}
	s.config = config
	return s.revision, nil
}
