package app

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	domaintypes "github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/iw2rmb/ploy/internal/testutil/clienv"
	"github.com/iw2rmb/ploy/internal/testutil/gitrepo"
)

func TestRunSubmitSelectorCases(t *testing.T) {
	t.Setenv("USER", "test-user")

	specPath := writeRunSubmitSpec(t, "steps:\n  - image: alpine:latest\n    command: echo hello\n")
	localRepo := gitrepo.SetupWithRemote(t, "https://gitlab.example.com/acme/local.git")
	localSHA := gitrepo.RevParse(t, localRepo, "HEAD")
	remoteSHA := "0123456789abcdef0123456789abcdef01234567"

	tests := []struct {
		name            string
		selector        string
		resolveResponse map[string]any
		wantResolve     map[string]string
		wantRepoURL     string
		wantRef         string
		wantCommitSHA   string
	}{
		{
			name:     "remote ref selector",
			selector: "acme/service:feature/test",
			resolveResponse: map[string]any{
				"repo_url":   "https://gitlab.example.com/acme/service.git",
				"ref":        "feature/test",
				"ref_is_sha": false,
			},
			wantResolve: map[string]string{"selector": "acme/service", "ref": "feature/test"},
			wantRepoURL: "https://gitlab.example.com/acme/service.git",
			wantRef:     "feature/test",
		},
		{
			name:     "remote sha selector",
			selector: "acme/service:" + remoteSHA,
			resolveResponse: map[string]any{
				"repo_url":   "https://gitlab.example.com/acme/service.git",
				"ref":        remoteSHA,
				"ref_is_sha": true,
			},
			wantResolve:   map[string]string{"selector": "acme/service", "ref": remoteSHA},
			wantRepoURL:   "https://gitlab.example.com/acme/service.git",
			wantRef:       remoteSHA,
			wantCommitSHA: remoteSHA,
		},
		{
			name:          "local repo selector",
			selector:      localRepo,
			wantRepoURL:   "https://gitlab.example.com/acme/local.git",
			wantRef:       localSHA,
			wantCommitSHA: localSHA,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runID := domaintypes.NewRunID().String()
			migID := domaintypes.NewMigID().String()
			specID := domaintypes.NewSpecID().String()
			var capturedResolve map[string]any
			var capturedSubmit map[string]any

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/v1/repos/resolve":
					if tc.wantResolve == nil {
						t.Fatalf("remote resolver should not be called for local selector")
					}
					if err := json.NewDecoder(r.Body).Decode(&capturedResolve); err != nil {
						t.Fatalf("decode resolve request: %v", err)
					}
					_ = json.NewEncoder(w).Encode(tc.resolveResponse)
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
			clienv.UseControlPlaneEnv(t, server.URL)

			var buf bytes.Buffer
			if err := executeCmd([]string{"run", specPath, tc.selector}, &buf); err != nil {
				t.Fatalf("run submit error: %v", err)
			}

			if tc.wantResolve != nil {
				if capturedResolve["selector"] != tc.wantResolve["selector"] || capturedResolve["ref"] != tc.wantResolve["ref"] {
					t.Fatalf("unexpected resolve request: %#v", capturedResolve)
				}
			}
			if capturedSubmit["repo_url"] != tc.wantRepoURL {
				t.Fatalf("repo_url = %v", capturedSubmit["repo_url"])
			}
			if capturedSubmit["ref"] != tc.wantRef {
				t.Fatalf("ref = %v", capturedSubmit["ref"])
			}
			if tc.wantCommitSHA == "" {
				if _, ok := capturedSubmit["commit_sha"]; ok {
					t.Fatalf("submit request must not contain commit_sha: %#v", capturedSubmit)
				}
			} else if capturedSubmit["commit_sha"] != tc.wantCommitSHA {
				t.Fatalf("commit_sha = %v", capturedSubmit["commit_sha"])
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
		})
	}
}
