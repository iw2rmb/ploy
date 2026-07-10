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

const namedResolvedSpecID = "spec1234"

func TestRunSubmitSpecSelectionCases(t *testing.T) {
	t.Setenv("USER", "test-user")

	tests := []struct {
		name                string
		specArg             func(t *testing.T) string
		resolveStatus       int
		resolveBody         any
		wantResolveSelector string
		wantResolveSHA      string
		wantCommand         string
		wantStepName        string
		wantStepCount       int
		wantBuildGate       bool
		wantRepoResolve     map[string]string
		wantErr             string
	}{
		{
			name: "local file path precedence",
			specArg: func(t *testing.T) string {
				t.Helper()
				specPath := filepath.Join(t.TempDir(), "upgrade-java")
				if err := os.WriteFile(specPath, []byte("steps:\n  - image: alpine:latest\n    command: echo local file\n"), 0o644); err != nil {
					t.Fatalf("write spec file: %v", err)
				}
				return specPath
			},
			wantCommand:     "echo local file",
			wantStepCount:   1,
			wantRepoResolve: map[string]string{"selector": "acme/target", "ref": "feature/run"},
		},
		{
			name: "local directory precedence uses mig yaml",
			specArg: func(t *testing.T) string {
				t.Helper()
				specDir := t.TempDir()
				if err := os.WriteFile(filepath.Join(specDir, "mig.yaml"), []byte("steps:\n  - image: alpine:latest\n    command: echo dir\n"), 0o644); err != nil {
					t.Fatalf("write mig.yaml: %v", err)
				}
				return specDir
			},
			wantCommand:     "echo dir",
			wantStepCount:   1,
			wantRepoResolve: map[string]string{"selector": "acme/target", "ref": "feature/run"},
		},
		{
			name: "local step selector submits selected step",
			specArg: func(t *testing.T) string {
				t.Helper()
				specPath := filepath.Join(t.TempDir(), "spec.yaml")
				if err := os.WriteFile(specPath, []byte(`
steps:
  - name: bootstrap
    image: alpine:latest
    command: echo bootstrap
  - name: deprecations
    image: alpine:latest
    command: echo deprecations
build_gate:
  disabled: false
`), 0o644); err != nil {
					t.Fatalf("write spec file: %v", err)
				}
				return specPath + ":deprecations"
			},
			wantCommand:     "echo deprecations",
			wantStepName:    "deprecations",
			wantStepCount:   1,
			wantBuildGate:   true,
			wantRepoResolve: map[string]string{"selector": "acme/target", "ref": "feature/run"},
		},
		{
			name:                "name only named selector",
			specArg:             func(t *testing.T) string { return "upgrade-java" },
			resolveStatus:       http.StatusOK,
			resolveBody:         namedRunSpecResolveBody("echo named"),
			wantResolveSelector: "upgrade-java",
			wantCommand:         "echo named",
			wantStepCount:       1,
			wantRepoResolve:     map[string]string{"selector": "acme/target", "ref": "feature/run"},
		},
		{
			name:                "repo name named selector",
			specArg:             func(t *testing.T) string { return "acme/specs:upgrade-java" },
			resolveStatus:       http.StatusOK,
			resolveBody:         namedRunSpecResolveBody("echo repo named"),
			wantResolveSelector: "acme/specs:upgrade-java",
			wantCommand:         "echo repo named",
			wantStepCount:       1,
			wantRepoResolve:     map[string]string{"selector": "acme/target", "ref": "feature/run"},
		},
		{
			name:                "domain repo name named selector",
			specArg:             func(t *testing.T) string { return "gitlab.example.com/acme/specs:upgrade-java" },
			resolveStatus:       http.StatusOK,
			resolveBody:         namedRunSpecResolveBody("echo domain named"),
			wantResolveSelector: "gitlab.example.com/acme/specs:upgrade-java",
			wantCommand:         "echo domain named",
			wantStepCount:       1,
			wantRepoResolve:     map[string]string{"selector": "acme/target", "ref": "feature/run"},
		},
		{
			name:                "versioned named selector",
			specArg:             func(t *testing.T) string { return "upgrade-java@01234567" },
			resolveStatus:       http.StatusOK,
			resolveBody:         namedRunSpecResolveBody("echo versioned"),
			wantResolveSelector: "upgrade-java",
			wantResolveSHA:      "01234567",
			wantCommand:         "echo versioned",
			wantStepCount:       1,
			wantRepoResolve:     map[string]string{"selector": "acme/target", "ref": "feature/run"},
		},
		{
			name:                "ambiguous named selector returns server message before run submit",
			specArg:             func(t *testing.T) string { return "upgrade-java" },
			resolveStatus:       http.StatusConflict,
			resolveBody:         map[string]string{"error": "named spec selector upgrade-java is ambiguous: github.com/acme/specs:upgrade-java, gitlab.example.com/acme/specs:upgrade-java"},
			wantResolveSelector: "upgrade-java",
			wantErr:             "run submit: named spec selector upgrade-java is ambiguous",
		},
		{
			name:                "invalid named selector returns server message before run submit",
			specArg:             func(t *testing.T) string { return "Bad" },
			resolveStatus:       http.StatusBadRequest,
			resolveBody:         map[string]string{"error": "invalid named spec selector: Bad"},
			wantResolveSelector: "Bad",
			wantErr:             "run submit: invalid named spec selector: Bad",
		},
		{
			name:                "unknown named selector returns not found before run submit",
			specArg:             func(t *testing.T) string { return "missing-spec" },
			resolveStatus:       http.StatusNotFound,
			resolveBody:         map[string]string{"error": "ignored not found body"},
			wantResolveSelector: "missing-spec",
			wantErr:             "run submit: named spec not found: missing-spec",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runID := domaintypes.NewRunID().String()
			migID := domaintypes.NewMigID().String()
			specID := domaintypes.NewSpecID().String()
			var capturedSpecResolveSelector string
			var capturedSpecResolveSHA string
			var capturedRepoResolve map[string]any
			var capturedSubmit map[string]any
			runSubmitCalled := false

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/v1/specs/resolve":
					if tc.wantResolveSelector == "" {
						t.Fatalf("named spec resolver should not be called for local selector")
					}
					capturedSpecResolveSelector = r.URL.Query().Get("selector")
					capturedSpecResolveSHA = r.URL.Query().Get("sha")
					status := tc.resolveStatus
					if status == 0 {
						status = http.StatusOK
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					_ = json.NewEncoder(w).Encode(tc.resolveBody)
				case r.Method == http.MethodPost && r.URL.Path == "/v1/repos/resolve":
					if tc.wantRepoResolve == nil {
						t.Fatalf("repo resolver should not be called")
					}
					if err := json.NewDecoder(r.Body).Decode(&capturedRepoResolve); err != nil {
						t.Fatalf("decode repo resolve request: %v", err)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{
						"repo_url":   "https://gitlab.example.com/acme/target.git",
						"ref":        "feature/run",
						"ref_is_sha": false,
					})
				case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
					runSubmitCalled = true
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
			clienv.UseControlPlaneEnv(t, server.URL)

			var buf bytes.Buffer
			err := executeCmd([]string{"run", tc.specArg(t), "acme/target:feature/run"}, &buf)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %q, want contains %q", err.Error(), tc.wantErr)
				}
				if runSubmitCalled {
					t.Fatalf("run submit should not be called after spec resolution error")
				}
				if capturedSpecResolveSelector != tc.wantResolveSelector {
					t.Fatalf("spec resolve selector = %q, want %q", capturedSpecResolveSelector, tc.wantResolveSelector)
				}
				return
			}
			if err != nil {
				t.Fatalf("run submit error: %v", err)
			}
			if tc.wantResolveSelector != "" && capturedSpecResolveSelector != tc.wantResolveSelector {
				t.Fatalf("spec resolve selector = %q, want %q", capturedSpecResolveSelector, tc.wantResolveSelector)
			}
			if capturedSpecResolveSHA != tc.wantResolveSHA {
				t.Fatalf("spec resolve sha = %q, want %q", capturedSpecResolveSHA, tc.wantResolveSHA)
			}
			if tc.wantRepoResolve != nil {
				if capturedRepoResolve["selector"] != tc.wantRepoResolve["selector"] || capturedRepoResolve["ref"] != tc.wantRepoResolve["ref"] {
					t.Fatalf("repo resolve request = %#v, want %#v", capturedRepoResolve, tc.wantRepoResolve)
				}
			}
			if !runSubmitCalled {
				t.Fatalf("expected run submit to be called")
			}
			spec := capturedSubmitSpec(t, capturedSubmit)
			steps := capturedSubmitSteps(t, capturedSubmit)
			if len(steps) != tc.wantStepCount {
				t.Fatalf("steps = %#v, want %d steps", spec["steps"], tc.wantStepCount)
			}
			step := steps[0].(map[string]any)
			if step["command"] != tc.wantCommand {
				t.Fatalf("step command = %v, want %q", step["command"], tc.wantCommand)
			}
			if tc.wantStepName != "" && step["name"] != tc.wantStepName {
				t.Fatalf("step name = %v, want %q", step["name"], tc.wantStepName)
			}
			if tc.wantBuildGate {
				buildGate, ok := spec["build_gate"].(map[string]any)
				if !ok || buildGate["disabled"] != false {
					t.Fatalf("build_gate = %#v, want disabled=false preserved", spec["build_gate"])
				}
			}
			if capturedSubmit["repo_url"] != "https://gitlab.example.com/acme/target.git" {
				t.Fatalf("repo_url = %v", capturedSubmit["repo_url"])
			}
			if capturedSubmit["ref"] != "feature/run" {
				t.Fatalf("ref = %v", capturedSubmit["ref"])
			}
			if tc.wantResolveSelector != "" {
				if capturedSubmit["spec_id"] != namedResolvedSpecID {
					t.Fatalf("spec_id = %v, want %s", capturedSubmit["spec_id"], namedResolvedSpecID)
				}
			} else if _, ok := capturedSubmit["spec_id"]; ok {
				t.Fatalf("local submit request must not contain spec_id: %#v", capturedSubmit)
			}
			if !strings.Contains(buf.String(), "run_id: "+runID) || !strings.Contains(buf.String(), "mig_id: "+migID) {
				t.Fatalf("unexpected output: %q", buf.String())
			}
		})
	}
}

func namedRunSpecResolveBody(command string) map[string]any {
	return map[string]any{
		"id":   namedResolvedSpecID,
		"name": "upgrade-java",
		"spec": map[string]any{
			"steps": []map[string]any{{
				"image":   "alpine:latest",
				"command": command,
			}},
		},
	}
}
