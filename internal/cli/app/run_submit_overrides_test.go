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

func TestRunSubmitEnvOverrides(t *testing.T) {
	t.Setenv("USER", "test-user")

	tests := []struct {
		name         string
		specArg      func(t *testing.T) string
		args         []string
		wantSelector string
		wantStepEnv  map[int]map[string]any
	}{
		{
			name: "named spec sends ordered env overrides to server",
			specArg: func(t *testing.T) string {
				return "upgrade-java"
			},
			args:         []string{"--env:build", "A=1", "--env:build", "A=2", "--env:test", "EMPTY="},
			wantSelector: "upgrade-java",
		},
		{
			name: "local spec submits overridden anonymous spec",
			specArg: func(t *testing.T) string {
				return writeRunSubmitSpec(t, "steps:\n  - name: build\n    image: alpine:latest\n")
			},
			args: []string{"--env:build", "A=1"},
			wantStepEnv: map[int]map[string]any{
				0: {"A": "1"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runID := domaintypes.NewRunID().String()
			migID := domaintypes.NewMigID().String()
			specID := domaintypes.NewSpecID().String()
			var capturedSubmit map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/v1/repos/resolve":
					_ = json.NewEncoder(w).Encode(map[string]any{
						"repo_url":   "https://gitlab.example.com/acme/target.git",
						"ref":        "main",
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
			args := append([]string{"run", tc.specArg(t), "acme/target"}, tc.args...)
			if err := executeCmd(args, &buf); err != nil {
				t.Fatalf("run submit error: %v", err)
			}
			if tc.wantSelector != "" {
				if capturedSubmit["spec_selector"] != tc.wantSelector {
					t.Fatalf("spec_selector = %v, want %q", capturedSubmit["spec_selector"], tc.wantSelector)
				}
				overrides := capturedSubmit["spec_overrides"].(map[string]any)
				stepEnvs := overrides["step_envs"].(map[string]any)
				if got := stepEnvs["build"].([]any); len(got) != 2 || got[0] != "A=1" || got[1] != "A=2" {
					t.Fatalf("build overrides = %#v, want ordered assignments", got)
				}
				if got := stepEnvs["test"].([]any); len(got) != 1 || got[0] != "EMPTY=" {
					t.Fatalf("test overrides = %#v, want empty assignment", got)
				}
			} else {
				steps := capturedSubmitSteps(t, capturedSubmit)
				for idx, wantEnv := range tc.wantStepEnv {
					envs, ok := steps[idx].(map[string]any)["envs"].(map[string]any)
					if !ok {
						t.Fatalf("steps[%d].envs = %#v, want object", idx, steps[idx].(map[string]any)["envs"])
					}
					for key, wantValue := range wantEnv {
						if envs[key] != wantValue {
							t.Fatalf("steps[%d].envs[%s] = %v, want %v", idx, key, envs[key], wantValue)
						}
					}
				}
			}
		})
	}
}

func TestRunSubmitBuildGateForcedOverride(t *testing.T) {
	t.Setenv("USER", "test-user")

	tests := []struct {
		name          string
		specArg       func(t *testing.T) string
		args          []string
		wantNamed     bool
		wantDisabled  bool
		wantImages    bool
		wantPreStack  map[string]any
		wantPostStack map[string]any
	}{
		{
			name: "global flag writes pre and post and re-enables disabled build gate",
			specArg: func(t *testing.T) string {
				spec := `
steps:
  - image: alpine:latest
build_gate:
  disabled: true
  images:
    - stack:
        language: java
        release: "17"
      image: gate-java17
`
				return writeRunSubmitSpec(t, spec)
			},
			args:          []string{"--build-gate-forced", "java@17/maven"},
			wantDisabled:  false,
			wantImages:    true,
			wantPreStack:  map[string]any{"mode": "forced", "language": "java", "release": "17", "tool": "maven"},
			wantPostStack: map[string]any{"mode": "forced", "language": "java", "release": "17", "tool": "maven"},
		},
		{
			name: "phase flags can be combined and only affect their phases",
			specArg: func(t *testing.T) string {
				spec := `
steps:
  - image: alpine:latest
build_gate:
  post:
    stack:
      mode: strict
      language: java
      release: "11"
`
				return writeRunSubmitSpec(t, spec)
			},
			args:          []string{"--build-gate-forced-pre", "java@21/gradle", "--build-gate-forced-post", "java@17"},
			wantDisabled:  false,
			wantPreStack:  map[string]any{"mode": "forced", "language": "java", "release": "21", "tool": "gradle"},
			wantPostStack: map[string]any{"mode": "forced", "language": "java", "release": "17"},
		},
		{
			name: "pre flag preserves untouched post phase",
			specArg: func(t *testing.T) string {
				spec := `
steps:
  - image: alpine:latest
build_gate:
  post:
    stack:
      mode: strict
      language: java
      release: "11"
`
				return writeRunSubmitSpec(t, spec)
			},
			args:          []string{"--build-gate-forced-pre", "java@21/gradle"},
			wantDisabled:  false,
			wantPreStack:  map[string]any{"mode": "forced", "language": "java", "release": "21", "tool": "gradle"},
			wantPostStack: map[string]any{"mode": "strict", "language": "java", "release": "11"},
		},
		{
			name:         "named spec sends build gate override to server",
			specArg:      func(t *testing.T) string { return "upgrade-java" },
			args:         []string{"--build-gate-forced-pre", "java@17/maven"},
			wantNamed:    true,
			wantPreStack: map[string]any{"language": "java", "release": "17", "tool": "maven"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedSubmit map[string]any
			server := newBuildGateForcedOverrideServer(t, &capturedSubmit)
			defer server.Close()
			clienv.UseControlPlaneEnv(t, server.URL)

			var buf bytes.Buffer
			args := append([]string{"run"}, tc.args...)
			args = append(args, tc.specArg(t), "acme/target")
			if err := executeCmd(args, &buf); err != nil {
				t.Fatalf("run submit error: %v", err)
			}
			if tc.wantNamed {
				if capturedSubmit["spec_selector"] != "upgrade-java" {
					t.Fatalf("spec_selector = %v, want upgrade-java", capturedSubmit["spec_selector"])
				}
				overrides := capturedSubmit["spec_overrides"].(map[string]any)
				forced := overrides["build_gate_forced"].(map[string]any)
				pre := forced["pre"].(map[string]any)
				for key, want := range tc.wantPreStack {
					if pre[key] != want {
						t.Fatalf("pre.%s = %v, want %v", key, pre[key], want)
					}
				}
				return
			}
			buildGate := capturedSubmitBuildGate(t, capturedSubmit)
			if buildGate["disabled"] != tc.wantDisabled {
				t.Fatalf("build_gate.disabled = %v, want %v", buildGate["disabled"], tc.wantDisabled)
			}
			if tc.wantImages {
				images, ok := buildGate["images"].([]any)
				if !ok || len(images) != 1 {
					t.Fatalf("build_gate.images = %#v, want one preserved rule", buildGate["images"])
				}
			}
			assertBuildGateStack(t, buildGate, "pre", tc.wantPreStack)
			assertBuildGateStack(t, buildGate, "post", tc.wantPostStack)
		})
	}
}

func newBuildGateForcedOverrideServer(t *testing.T, capturedSubmit *map[string]any) *httptest.Server {
	t.Helper()
	runID := domaintypes.NewRunID().String()
	migID := domaintypes.NewMigID().String()
	specID := domaintypes.NewSpecID().String()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/repos/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repo_url":   "https://gitlab.example.com/acme/target.git",
				"ref":        "main",
				"ref_is_sha": false,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			if err := json.NewDecoder(r.Body).Decode(capturedSubmit); err != nil {
				t.Fatalf("decode submit request: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"run_id": runID, "mig_id": migID, "spec_id": specID})
		default:
			http.NotFound(w, r)
		}
	}))
}

func assertBuildGateStack(t *testing.T, buildGate map[string]any, phase string, want map[string]any) {
	t.Helper()
	if want == nil {
		return
	}
	phaseMap, ok := buildGate[phase].(map[string]any)
	if !ok {
		t.Fatalf("build_gate.%s = %#v, want object", phase, buildGate[phase])
	}
	stack, ok := phaseMap["stack"].(map[string]any)
	if !ok {
		t.Fatalf("build_gate.%s.stack = %#v, want object", phase, phaseMap["stack"])
	}
	for key, wantValue := range want {
		if stack[key] != wantValue {
			t.Fatalf("build_gate.%s.stack.%s = %v, want %v", phase, key, stack[key], wantValue)
		}
	}
	if _, wantTool := want["tool"]; !wantTool {
		if _, ok := stack["tool"]; ok {
			t.Fatalf("build_gate.%s.stack.tool present = %v, want omitted", phase, stack["tool"])
		}
	}
}

func TestRunSubmitBuildGateForcedFlagValidation(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "global plus phase-specific",
			args:    []string{"run", "--build-gate-forced", "java@17", "--build-gate-forced-pre", "java@21", "spec.yaml", "acme/target"},
			wantErr: "--build-gate-forced cannot be combined",
		},
		{
			name:    "missing separator",
			args:    []string{"run", "--build-gate-forced", "java17", "spec.yaml", "acme/target"},
			wantErr: `invalid --build-gate-forced value "java17": expected <lang>@<release>[/<tool>]`,
		},
		{
			name:    "empty tool",
			args:    []string{"run", "--build-gate-forced-post", "java@17/", "spec.yaml", "acme/target"},
			wantErr: `invalid --build-gate-forced-post value "java@17/": expected <lang>@<release>[/<tool>]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := executeCmd(tt.args, &buf)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestRunSubmitEnvOverrideValidation(t *testing.T) {
	t.Setenv("USER", "test-user")
	tests := []struct {
		name    string
		spec    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing step",
			spec:    "steps:\n  - name: build\n    image: alpine:latest\n",
			args:    []string{"--env:test", "A=1"},
			wantErr: `step "test" not found`,
		},
		{
			name:    "duplicate step",
			spec:    "steps:\n  - name: build\n    image: alpine:latest\n  - name: build\n    image: alpine:latest\n",
			args:    []string{"--env:build", "A=1"},
			wantErr: `step name "build" is not unique`,
		},
		{
			name:    "malformed assignment",
			spec:    "steps:\n  - name: build\n    image: alpine:latest\n",
			args:    []string{"--env:build", "A"},
			wantErr: "must be KEY=VALUE",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			specPath := writeRunSubmitSpec(t, tt.spec)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == "/v1/repos/resolve" {
					_ = json.NewEncoder(w).Encode(map[string]any{"repo_url": "https://gitlab.example.com/acme/target.git", "ref": "main", "ref_is_sha": false})
					return
				}
				t.Fatalf("unexpected request after env validation failure: %s %s", r.Method, r.URL.Path)
			}))
			defer server.Close()
			clienv.UseControlPlaneEnv(t, server.URL)

			var buf bytes.Buffer
			args := append([]string{"run", specPath, "acme/target"}, tt.args...)
			err := executeCmd(args, &buf)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
