package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/cli/controlplane"
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
	gitlabConfigStoreFactory = func(_ context.Context, _ controlplane.Options) (gitlabStore, error) {
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
	gitlabConfigStoreFactory = func(_ context.Context, _ controlplane.Options) (gitlabStore, error) {
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

func TestConfigGitlabUsageMatchesGolden(t *testing.T) {
	t.Helper()
	buf := &bytes.Buffer{}
	err := execute([]string{"config", "gitlab"}, buf)
	if err == nil {
		t.Fatal("expected error for missing gitlab subcommand")
	}
	expect := loadConfigGolden(t, "config_gitlab_usage.txt")
	if diff := diffStringsLocal(expect, buf.String()); diff != "" {
		t.Fatalf("gitlab usage mismatch:\n%s", diff)
	}
}

func TestConfigGitlabStatusOutputsSignerSummary(t *testing.T) {
	t.Helper()
	originalFactory := gitlabSignerClientFactory
	t.Cleanup(func() { gitlabSignerClientFactory = originalFactory })

	rotatedAt := time.Date(2025, time.October, 21, 15, 4, 5, 0, time.UTC)
	stub := &stubGitlabSignerOps{
		status: gitlabSignerStatus{
			FeedRevision: 128,
			Secrets: []gitlabSignerSecretStatus{
				{
					Name:      "runner",
					Revision:  64,
					Scopes:    []string{"api", "read_repository"},
					RotatedAt: rotatedAt,
					Audit: gitlabSignerAudit{
						LastRotation: rotatedAt,
						Revocations: []gitlabSignerRevocation{
							{NodeID: "node-a", TokenID: "tok-1", Timestamp: rotatedAt.Add(2 * time.Minute)},
						},
						Failures: []gitlabSignerFailure{
							{NodeID: "node-b", TokenID: "tok-2", Timestamp: rotatedAt.Add(3 * time.Minute), Error: "gitlab timeout"},
						},
					},
				},
			},
		},
	}
	gitlabSignerClientFactory = func(_ context.Context, _ controlplane.Options) (gitlabSignerClient, error) {
		return stub, nil
	}

	buf := &bytes.Buffer{}
	if err := execute([]string{"config", "gitlab", "status"}, buf); err != nil {
		t.Fatalf("execute status: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Secret: runner") {
		t.Fatalf("expected runner secret in output, got %q", output)
	}
	if !strings.Contains(output, "Scopes: api, read_repository") {
		t.Fatalf("expected scopes in output, got %q", output)
	}
	if !strings.Contains(output, "Audit feed revision: 128") {
		t.Fatalf("expected feed revision in output, got %q", output)
	}
	if !strings.Contains(output, "node-b") || !strings.Contains(output, "gitlab timeout") {
		t.Fatalf("expected failure details in output, got %q", output)
	}
	if !stub.closed {
		t.Fatal("expected signer client to be closed")
	}
}

func TestConfigGitlabRotateTriggersSecretUpdate(t *testing.T) {
	t.Helper()
	originalFactory := gitlabSignerClientFactory
	t.Cleanup(func() { gitlabSignerClientFactory = originalFactory })

	stub := &stubGitlabSignerOps{}
	gitlabSignerClientFactory = func(_ context.Context, _ controlplane.Options) (gitlabSignerClient, error) {
		return stub, nil
	}

	buf := &bytes.Buffer{}
	err := execute([]string{
		"config", "gitlab", "rotate",
		"--secret", "runner",
		"--api-key", "glpat-123",
		"--scope", "api",
		"--scope", "read_repository",
	}, buf)
	if err != nil {
		t.Fatalf("execute rotate: %v", err)
	}
	if !stub.rotated {
		t.Fatal("expected rotate to be invoked")
	}
	if stub.rotateReq.Secret != "runner" {
		t.Fatalf("unexpected secret: %+v", stub.rotateReq)
	}
	if len(stub.rotateReq.Scopes) != 2 || stub.rotateReq.Scopes[1] != "read_repository" {
		t.Fatalf("unexpected scopes: %+v", stub.rotateReq)
	}
	if !strings.Contains(buf.String(), "GitLab secret runner rotated") {
		t.Fatalf("expected rotation success message, got %q", buf.String())
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

type stubGitlabSignerOps struct {
	status    gitlabSignerStatus
	statusErr error

	rotated   bool
	rotateReq gitlabRotateSecretRequest
	rotateRes gitlabRotateSecretResult
	rotateErr error

	closed bool
}

func (s *stubGitlabSignerOps) Status(context.Context, gitlabSignerStatusRequest) (gitlabSignerStatus, error) {
	return s.status, s.statusErr
}

func (s *stubGitlabSignerOps) RotateSecret(_ context.Context, req gitlabRotateSecretRequest) (gitlabRotateSecretResult, error) {
	s.rotated = true
	s.rotateReq = req
	if s.rotateErr != nil {
		return gitlabRotateSecretResult{}, s.rotateErr
	}
	if s.rotateRes.Secret == "" {
		s.rotateRes.Secret = req.Secret
	}
	return s.rotateRes, nil
}

func (s *stubGitlabSignerOps) Close() error {
	s.closed = true
	return nil
}

func loadConfigGolden(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	return string(data)
}

func diffStringsLocal(expect, actual string) string {
	if expect == actual {
		return ""
	}
	return "expected:\n" + expect + "\nactual:\n" + actual
}
