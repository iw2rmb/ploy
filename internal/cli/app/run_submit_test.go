package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
)

func TestRunSubmitRemoteSelectorCallsControlPlane(t *testing.T) {
	t.Setenv("USER", "test-user")

	specPath := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(specPath, []byte("steps:\n  - image: alpine:latest\n    command: echo hello\n"), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}

	runID := domaintypes.NewRunID().String()
	migID := domaintypes.NewMigID().String()
	specID := domaintypes.NewSpecID().String()
	var capturedResolve map[string]any
	var capturedSubmit map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/repos/resolve":
			if err := json.NewDecoder(r.Body).Decode(&capturedResolve); err != nil {
				t.Fatalf("decode resolve request: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repo_url":   "https://gitlab.example.com/acme/service.git",
				"ref":        "feature/test",
				"ref_is_sha": false,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			if err := json.NewDecoder(r.Body).Decode(&capturedSubmit); err != nil {
				t.Fatalf("decode submit request: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"run_id":  runID,
				"mig_id":  migID,
				"spec_id": specID,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	clienv.UseServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	if err := executeCmd([]string{"run", specPath, "acme/service:feature/test"}, &buf); err != nil {
		t.Fatalf("run submit error: %v", err)
	}

	if capturedResolve["selector"] != "acme/service" || capturedResolve["ref"] != "feature/test" {
		t.Fatalf("unexpected resolve request: %#v", capturedResolve)
	}
	if capturedSubmit["repo_url"] != "https://gitlab.example.com/acme/service.git" {
		t.Fatalf("repo_url = %v", capturedSubmit["repo_url"])
	}
	if capturedSubmit["ref"] != "feature/test" {
		t.Fatalf("ref = %v", capturedSubmit["ref"])
	}
	if _, ok := capturedSubmit["base_ref"]; ok {
		t.Fatalf("submit request must not contain base_ref: %#v", capturedSubmit)
	}
	if capturedSubmit["created_by"] != "test-user" {
		t.Fatalf("created_by = %v", capturedSubmit["created_by"])
	}
	if !strings.Contains(buf.String(), "run_id: "+runID) || !strings.Contains(buf.String(), "mig_id: "+migID) {
		t.Fatalf("unexpected output: %q", buf.String())
	}
}

func TestRunSubmitSpecDirectoryUsesMigYAML(t *testing.T) {
	specDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(specDir, "mig.yaml"), []byte("steps:\n  - image: alpine:latest\n    command: echo dir\n"), 0o644); err != nil {
		t.Fatalf("write mig.yaml: %v", err)
	}

	var capturedSubmit map[string]any
	runID := domaintypes.NewRunID().String()
	migID := domaintypes.NewMigID().String()
	specID := domaintypes.NewSpecID().String()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/repos/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repo_url":   "https://gitlab.example.com/acme/service.git",
				"ref":        "master",
				"ref_is_sha": false,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			if err := json.NewDecoder(r.Body).Decode(&capturedSubmit); err != nil {
				t.Fatalf("decode submit request: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"run_id": runID, "mig_id": migID, "spec_id": specID})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	clienv.UseServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	if err := executeCmd([]string{"run", specDir, "acme/service"}, &buf); err != nil {
		t.Fatalf("run submit dir spec error: %v", err)
	}
	spec, ok := capturedSubmit["spec"].(map[string]any)
	if !ok {
		t.Fatalf("expected spec object, got %#v", capturedSubmit["spec"])
	}
	steps := spec["steps"].([]any)
	step := steps[0].(map[string]any)
	if step["command"] != "echo dir" {
		t.Fatalf("expected mig.yaml content, got %#v", step)
	}
}

func TestRunSubmitRemoteSHAUsesCommitSHA(t *testing.T) {
	specPath := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(specPath, []byte("steps:\n  - image: alpine\n"), 0o644); err != nil {
		t.Fatalf("write spec file: %v", err)
	}
	sha := "0123456789abcdef0123456789abcdef01234567"
	var capturedSubmit map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/repos/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repo_url":   "https://gitlab.example.com/acme/service.git",
				"ref":        sha,
				"ref_is_sha": true,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			if err := json.NewDecoder(r.Body).Decode(&capturedSubmit); err != nil {
				t.Fatalf("decode submit request: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"run_id":  domaintypes.NewRunID().String(),
				"mig_id":  domaintypes.NewMigID().String(),
				"spec_id": domaintypes.NewSpecID().String(),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	clienv.UseServerDescriptor(t, server.URL)

	var buf bytes.Buffer
	if err := executeCmd([]string{"run", specPath, "acme/service:" + sha}, &buf); err != nil {
		t.Fatalf("run submit sha error: %v", err)
	}
	if capturedSubmit["commit_sha"] != sha {
		t.Fatalf("commit_sha = %v", capturedSubmit["commit_sha"])
	}
	if capturedSubmit["ref"] != sha {
		t.Fatalf("ref = %v", capturedSubmit["ref"])
	}
}
