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
)

func TestRunSubmitGitLabTokenFlag(t *testing.T) {
	t.Setenv("USER", "test-user")

	specPath := writeRunSubmitSpec(t, "steps:\n  - image: alpine:latest\n    command: echo hello\n")
	restartRunID := domaintypes.NewRunID().String()

	tests := []struct {
		name       string
		args       []string
		stdin      string
		envName    string
		envValue   string
		wantToken  string
		wantErr    string
		wantCalled bool
	}{
		{
			name:       "run env sends token only",
			args:       []string{"run", "--gitlab-token-env", "PLOY_TEST_TOKEN", specPath, "acme/service"},
			envName:    "PLOY_TEST_TOKEN",
			envValue:   "glpat-env-secret",
			wantToken:  "glpat-env-secret",
			wantCalled: true,
		},
		{
			name:       "run prompt sends token only",
			args:       []string{"run", "--gitlab-token-prompt", specPath, "acme/service"},
			stdin:      "glpat-prompt-secret\n",
			wantToken:  "glpat-prompt-secret",
			wantCalled: true,
		},
		{
			name:    "missing env var rejected before submit",
			args:    []string{"run", "--gitlab-token-env", "PLOY_TEST_TOKEN_MISSING", specPath, "acme/service"},
			wantErr: "--gitlab-token-env PLOY_TEST_TOKEN_MISSING is not set or empty",
		},
		{
			name:     "empty env var rejected before submit",
			args:     []string{"run", "--gitlab-token-env", "PLOY_TEST_TOKEN", specPath, "acme/service"},
			envName:  "PLOY_TEST_TOKEN",
			envValue: "",
			wantErr:  "--gitlab-token-env PLOY_TEST_TOKEN is not set or empty",
		},
		{
			name:     "env and prompt rejected before submit",
			args:     []string{"run", "--gitlab-token-env", "PLOY_TEST_TOKEN", "--gitlab-token-prompt", specPath, "acme/service"},
			envName:  "PLOY_TEST_TOKEN",
			envValue: "glpat-env-secret",
			wantErr:  "--gitlab-token-env and --gitlab-token-prompt are mutually exclusive",
		},
		{
			name:    "old flag rejected",
			args:    []string{"run", "--gitlab-token=glpat-expanded-secret", specPath, "acme/service"},
			wantErr: "unknown flag: --gitlab-token",
		},
		{
			name:       "mig run env sends token only",
			args:       []string{"mig", "run", "--gitlab-token-env", "PLOY_TEST_TOKEN", "my-wave"},
			envName:    "PLOY_TEST_TOKEN",
			envValue:   "glpat-mig-env-secret",
			wantToken:  "glpat-mig-env-secret",
			wantCalled: true,
		},
		{
			name:       "run restart env sends token only",
			args:       []string{"run", "restart", "--gitlab-token-env", "PLOY_TEST_TOKEN", restartRunID},
			envName:    "PLOY_TEST_TOKEN",
			envValue:   "glpat-restart-env-secret",
			wantToken:  "glpat-restart-env-secret",
			wantCalled: true,
		},
		{
			name:       "run restart prompt sends token only",
			args:       []string{"run", "restart", "--gitlab-token-prompt", restartRunID},
			stdin:      "glpat-restart-prompt-secret\n",
			wantToken:  "glpat-restart-prompt-secret",
			wantCalled: true,
		},
		{
			name:    "run restart missing env var rejected before submit",
			args:    []string{"run", "restart", "--gitlab-token-env", "PLOY_TEST_TOKEN_MISSING", restartRunID},
			wantErr: "--gitlab-token-env PLOY_TEST_TOKEN_MISSING is not set or empty",
		},
		{
			name:     "run restart env and prompt rejected before submit",
			args:     []string{"run", "restart", "--gitlab-token-env", "PLOY_TEST_TOKEN", "--gitlab-token-prompt", restartRunID},
			envName:  "PLOY_TEST_TOKEN",
			envValue: "glpat-restart-env-secret",
			wantErr:  "--gitlab-token-env and --gitlab-token-prompt are mutually exclusive",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envName != "" {
				t.Setenv(tc.envName, tc.envValue)
			}
			if strings.Contains(strings.Join(tc.args, " "), "PLOY_TEST_TOKEN_MISSING") {
				t.Setenv("PLOY_TEST_TOKEN_MISSING", "")
			}
			runID := domaintypes.NewRunID().String()
			migID := domaintypes.NewMigID().String()
			specID := domaintypes.NewSpecID().String()
			waveID := domaintypes.NewWaveID().String()
			waveMigID := domaintypes.NewMigID()
			var capturedSubmit map[string]any
			runSubmitCalled := false

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/v1/migs":
					_ = json.NewEncoder(w).Encode(map[string]any{
						"migs": []map[string]any{{
							"id":       waveMigID.String(),
							"name":     "my-wave",
							"archived": false,
						}},
					})
				case r.Method == http.MethodPost && r.URL.Path == "/v1/repos/resolve":
					_ = json.NewEncoder(w).Encode(map[string]any{
						"repo_url":   "https://gitlab.example.com/acme/service.git",
						"ref":        "main",
						"ref_is_sha": false,
					})
				case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
					runSubmitCalled = true
					if err := json.NewDecoder(r.Body).Decode(&capturedSubmit); err != nil {
						t.Fatalf("decode submit request: %v", err)
					}
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"run_id":  runID,
						"mig_id":  migID,
						"spec_id": specID,
					})
				case r.Method == http.MethodPost && r.URL.Path == "/v1/migs/"+waveMigID.String()+"/waves":
					runSubmitCalled = true
					if err := json.NewDecoder(r.Body).Decode(&capturedSubmit); err != nil {
						t.Fatalf("decode wave request: %v", err)
					}
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"wave_id":   waveID,
						"mig_id":    waveMigID.String(),
						"spec_id":   specID,
						"run_count": 1,
					})
				case r.Method == http.MethodPost && r.URL.Path == "/v1/runs/"+restartRunID+"/restart":
					runSubmitCalled = true
					if err := json.NewDecoder(r.Body).Decode(&capturedSubmit); err != nil {
						t.Fatalf("decode restart request: %v", err)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":         restartRunID,
						"status":     "Queued",
						"mig_id":     migID,
						"spec_id":    specID,
						"attempt":    2,
						"created_at": "2026-07-10T00:00:00Z",
					})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			clienv.UseControlPlaneEnv(t, server.URL)

			var buf bytes.Buffer
			root := NewRootCmdWithIO(&buf, &buf)
			root.SetArgs(tc.args)
			if tc.stdin != "" {
				root.SetIn(strings.NewReader(tc.stdin))
			}
			err := root.Execute()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
				}
				if runSubmitCalled {
					t.Fatalf("run submit should not be called")
				}
				return
			}
			if err != nil {
				t.Fatalf("run submit error: %v", err)
			}
			if runSubmitCalled != tc.wantCalled {
				t.Fatalf("run submit called = %v, want %v", runSubmitCalled, tc.wantCalled)
			}
			if capturedSubmit["gitlab_token"] != tc.wantToken {
				t.Fatalf("gitlab_token = %v, want %q", capturedSubmit["gitlab_token"], tc.wantToken)
			}
			if _, ok := capturedSubmit["gitlab_token_hash"]; ok {
				t.Fatalf("submit request must not contain gitlab_token_hash: %#v", capturedSubmit)
			}
			if strings.Contains(buf.String(), tc.wantToken) {
				t.Fatalf("output leaked token: %q", buf.String())
			}
		})
	}
}
