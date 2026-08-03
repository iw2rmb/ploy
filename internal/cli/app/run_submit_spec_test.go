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

func TestRunSubmitSpecSelectionCases(t *testing.T) {
	t.Setenv("USER", "test-user")

	tests := []struct {
		name             string
		specArg          func(t *testing.T) string
		wantSpecSelector string
		wantCommand      string
		wantStepName     string
		wantStepCount    int
		wantBuildGate    bool
		wantRepoResolve  map[string]string
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
			name:             "name only named selector",
			specArg:          func(t *testing.T) string { return "upgrade-java" },
			wantSpecSelector: "upgrade-java",
			wantRepoResolve:  map[string]string{"selector": "acme/target", "ref": "feature/run"},
		},
		{
			name:             "repo name named selector",
			specArg:          func(t *testing.T) string { return "acme/specs:upgrade-java" },
			wantSpecSelector: "acme/specs:upgrade-java",
			wantRepoResolve:  map[string]string{"selector": "acme/target", "ref": "feature/run"},
		},
		{
			name:             "domain repo name named selector",
			specArg:          func(t *testing.T) string { return "gitlab.example.com/acme/specs:upgrade-java" },
			wantSpecSelector: "gitlab.example.com/acme/specs:upgrade-java",
			wantRepoResolve:  map[string]string{"selector": "acme/target", "ref": "feature/run"},
		},
		{
			name:             "versioned selector is sent for server validation",
			specArg:          func(t *testing.T) string { return "upgrade-java@01234567" },
			wantSpecSelector: "upgrade-java@01234567",
			wantRepoResolve:  map[string]string{"selector": "acme/target", "ref": "feature/run"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runID := domaintypes.NewRunID().String()
			migID := domaintypes.NewMigID().String()
			specID := domaintypes.NewSpecID().String()
			var capturedRepoResolve map[string]any
			var capturedSubmit map[string]any

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/v1/repos/resolve":
					if err := json.NewDecoder(r.Body).Decode(&capturedRepoResolve); err != nil {
						t.Fatalf("decode repo resolve request: %v", err)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{
						"repo_url":   "https://gitlab.example.com/acme/target.git",
						"ref":        "feature/run",
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
			clienv.UseControlPlaneEnv(t, server.URL)

			var buf bytes.Buffer
			err := executeCmd([]string{"run", tc.specArg(t), "acme/target:feature/run"}, &buf)
			if err != nil {
				t.Fatalf("run submit error: %v", err)
			}
			if tc.wantRepoResolve != nil {
				if capturedRepoResolve["selector"] != tc.wantRepoResolve["selector"] || capturedRepoResolve["ref"] != tc.wantRepoResolve["ref"] {
					t.Fatalf("repo resolve request = %#v, want %#v", capturedRepoResolve, tc.wantRepoResolve)
				}
			}
			if tc.wantSpecSelector == "" {
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
			} else {
				if capturedSubmit["spec_selector"] != tc.wantSpecSelector {
					t.Fatalf("spec_selector = %v, want %q", capturedSubmit["spec_selector"], tc.wantSpecSelector)
				}
				if _, ok := capturedSubmit["spec"]; ok {
					t.Fatalf("named submit request must not contain spec: %#v", capturedSubmit)
				}
			}
			if capturedSubmit["repo_url"] != "https://gitlab.example.com/acme/target.git" {
				t.Fatalf("repo_url = %v", capturedSubmit["repo_url"])
			}
			if capturedSubmit["ref"] != "feature/run" {
				t.Fatalf("ref = %v", capturedSubmit["ref"])
			}
			if !strings.Contains(buf.String(), "run_id: "+runID) || !strings.Contains(buf.String(), "mig_id: "+migID) {
				t.Fatalf("unexpected output: %q", buf.String())
			}
		})
	}
}
