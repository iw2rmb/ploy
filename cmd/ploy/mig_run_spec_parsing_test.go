package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// specPayloadOpts holds optional buildSpecPayload parameters.
// Zero values correspond to the default (nil/empty/false) arguments.
type specPayloadOpts struct {
	migEnvs      []string
	migImage     string
	retain       bool
	migCommand   string
	gitlabPAT    string
	gitlabDomain string
	mrSuccess    bool
	mrFail       bool
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", filepath.Base(path), err)
	}
}

func callBuildSpecPayload(t *testing.T, specFile string, opts specPayloadOpts) ([]byte, error) {
	t.Helper()
	return buildSpecPayload(
		context.Background(), nil, nil, specFile,
		opts.migEnvs, opts.migImage, opts.retain, opts.migCommand,
		opts.gitlabPAT, opts.gitlabDomain, opts.mrSuccess, opts.mrFail,
	)
}

// buildAndParseSpec writes specContent to dir/spec{ext}, calls buildSpecPayload,
// and returns the parsed JSON map. Use when the caller needs to write auxiliary
// files (fragments, env files) to the same directory before this call.
func buildAndParseSpec(t *testing.T, dir, specContent, ext string, opts specPayloadOpts) map[string]any {
	t.Helper()
	specFile := filepath.Join(dir, "spec"+ext)
	writeFile(t, specFile, specContent)
	payload, err := callBuildSpecPayload(t, specFile, opts)
	if err != nil {
		t.Fatalf("buildSpecPayload: %v", err)
	}
	return unmarshalPayload(t, payload)
}

// runBuildSpecPayload creates a temp dir (or passes empty specFile when
// specContent is empty), calls buildSpecPayload, and returns the parsed map.
func runBuildSpecPayload(t *testing.T, specContent, ext string, opts specPayloadOpts) map[string]any {
	t.Helper()
	if specContent == "" {
		payload, err := callBuildSpecPayload(t, "", opts)
		if err != nil {
			t.Fatalf("buildSpecPayload: %v", err)
		}
		return unmarshalPayload(t, payload)
	}
	return buildAndParseSpec(t, t.TempDir(), specContent, ext, opts)
}

func unmarshalPayload(t *testing.T, payload []byte) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return result
}

// mustDig traverses a nested map[string]any by key path, calling t.Fatalf
// on any missing or non-map key.
func mustDig(t *testing.T, m map[string]any, keys ...string) map[string]any {
	t.Helper()
	cur := m
	for i, k := range keys {
		next, ok := cur[k].(map[string]any)
		if !ok {
			t.Fatalf("expected %s to be map at path %v, got %T", k, keys[:i+1], cur[k])
		}
		cur = next
	}
	return cur
}

// mustSteps extracts the steps array, validates length, and returns typed entries.
func mustSteps(t *testing.T, result map[string]any, wantLen int) []map[string]any {
	t.Helper()
	raw, ok := result["steps"].([]any)
	if !ok {
		t.Fatalf("expected steps array, got %T", result["steps"])
	}
	if len(raw) != wantLen {
		t.Fatalf("expected %d steps, got %d", wantLen, len(raw))
	}
	out := make([]map[string]any, len(raw))
	for i, s := range raw {
		m, ok := s.(map[string]any)
		if !ok {
			t.Fatalf("steps[%d]: expected map, got %T", i, s)
		}
		out[i] = m
	}
	return out
}

func assertField(t *testing.T, m map[string]any, key string, want any) {
	t.Helper()
	got, ok := m[key]
	if !ok {
		t.Errorf("expected %s to exist", key)
		return
	}
	if got != want {
		t.Errorf("%s = %v (%T), want %v (%T)", key, got, got, want, want)
	}
}

func assertAbsent(t *testing.T, m map[string]any, key string) {
	t.Helper()
	if v, ok := m[key]; ok {
		t.Errorf("expected %s to be absent, got %v", key, v)
	}
}

func assertCommandArr(t *testing.T, step map[string]any, want []string) {
	t.Helper()
	cmd := step["command"]
	arr, ok := cmd.([]any)
	if !ok {
		t.Fatalf("expected command as []any, got %T", cmd)
	}
	if len(arr) != len(want) {
		t.Fatalf("command len = %d, want %d: %v", len(arr), len(want), arr)
	}
	for i, w := range want {
		if got, _ := arr[i].(string); got != w {
			t.Errorf("command[%d] = %q, want %q", i, got, w)
		}
	}
}

// ---------------------------------------------------------------------------
// Table-driven tests
// ---------------------------------------------------------------------------

func TestBuildSpecPayload_ErrorCases(t *testing.T) {
	// Not parallel: some sub-cases use t.Setenv.

	tests := []struct {
		name     string
		specFile string            // literal path (bypasses temp file creation)
		spec     string            // YAML content to write to temp file
		setenv   map[string]string // env vars; empty value "" → replaced with tmpDir
		wantErr  string            // exact match; empty → just assert err != nil
	}{
		{
			name:     "non-existent file",
			specFile: "/nonexistent/path/spec.yaml",
		},
		{
			name: "invalid YAML format",
			spec: "not: valid: yaml: content:",
		},
		{
			name: "include fragment must start with slash",
			spec: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  router: !include ./router.yaml#router
`,
		},
		{
			name: "include missing file",
			spec: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  router: !include ./missing-router.yaml#/router
`,
		},
		{
			name: "unresolved image env placeholder",
			spec: `
steps:
  - image: $PLOY_TEST_MISSING_IMAGE
`,
			wantErr: "resolve image env placeholders: steps[0].image: unresolved environment variables: PLOY_TEST_MISSING_IMAGE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.setenv) == 0 {
				t.Parallel()
			}

			var err error
			if tt.specFile != "" {
				_, err = callBuildSpecPayload(t, tt.specFile, specPayloadOpts{})
			} else {
				tmpDir := t.TempDir()
				for k, v := range tt.setenv {
					if v == "" {
						v = tmpDir
					}
					t.Setenv(k, v)
				}
				specPath := filepath.Join(tmpDir, "spec.yaml")
				writeFile(t, specPath, tt.spec)
				_, err = callBuildSpecPayload(t, specPath, specPayloadOpts{})
			}

			if err == nil {
				t.Fatal("expected error")
			}
			if tt.wantErr != "" && err.Error() != tt.wantErr {
				t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestBuildSpecPayload_CommandHandling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		spec       string
		opts       specPayloadOpts
		wantCmdArr []string
		wantCmdStr string
	}{
		{
			name: "spec command preserved when no CLI flag",
			spec: `
steps:
  - image: docker.io/test/mig:latest
    command: ["/bin/sh", "-lc", "echo hi"]
`,
			wantCmdArr: []string{"/bin/sh", "-lc", "echo hi"},
		},
		{
			name: "CLI JSON array overrides spec command",
			spec: `
steps:
  - image: docker.io/test/mig:latest
    command: ["echo", "spec"]
`,
			opts:       specPayloadOpts{migCommand: `["echo","cli"]`},
			wantCmdArr: []string{"echo", "cli"},
		},
		{
			name:       "CLI JSON array command without spec file",
			opts:       specPayloadOpts{migImage: "docker.io/test/mig:latest", migCommand: `["/bin/sh", "-c", "echo test"]`},
			wantCmdArr: []string{"/bin/sh", "-c", "echo test"},
		},
		{
			name:       "CLI plain string command without spec file",
			opts:       specPayloadOpts{migImage: "docker.io/test/mig:latest", migCommand: "echo test"},
			wantCmdStr: "echo test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := runBuildSpecPayload(t, tt.spec, ".yaml", tt.opts)
			steps := mustSteps(t, result, 1)
			if tt.wantCmdArr != nil {
				assertCommandArr(t, steps[0], tt.wantCmdArr)
			}
			if tt.wantCmdStr != "" {
				got, ok := steps[0]["command"].(string)
				if !ok || got != tt.wantCmdStr {
					t.Errorf("command = %v, want %q", steps[0]["command"], tt.wantCmdStr)
				}
			}
		})
	}
}

// digCheck describes a nested assertion: dig into the result at digPath,
// then verify wantFields and wantAbsent keys.
type digCheck struct {
	digPath    []string
	wantFields map[string]any
	wantAbsent []string
}

func TestBuildSpecPayload_BasicParsing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		spec          string
		ext           string
		wantStepImage string
		wantEnv       map[string]any
		wantRetries   float64
		wantDomain    string
		wantMRSuccess bool
		digChecks     []digCheck // optional nested field assertions
	}{
		{
			name: "YAML spec",
			ext:  ".yaml",
			spec: `
steps:
  - image: docker.io/test/mig:latest
envs:
  KEY1: value1
  KEY2: value2
build_gate:
  heal:
    retries: 1
    image: docker.io/test/healer:latest
gitlab_domain: gitlab.example.com
mr_on_success: true
`,
			wantStepImage: "docker.io/test/mig:latest",
			wantEnv:       map[string]any{"KEY1": "value1", "KEY2": "value2"},
			wantDomain:    "gitlab.example.com",
			wantMRSuccess: true,
		},
		{
			name: "JSON spec",
			ext:  ".json",
			spec: `{
  "steps": [{"image": "docker.io/test/mig:latest"}],
  "envs": {"KEY1": "value1"},
  "build_gate": {
    "heal": {
      "retries": 2,
      "image": "docker.io/test/healer:latest"
    }
  }
}`,
			wantStepImage: "docker.io/test/mig:latest",
			wantRetries:   2,
		},
		{
			name: "heal fields and envs",
			ext:  ".yaml",
			spec: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  heal:
    retries: 2
    image: docker.io/test/healer:latest
    command: "heal.sh"
    envs:
      HEALING_MODE: auto
`,
			wantStepImage: "docker.io/test/mig:latest",
			digChecks: []digCheck{
				{
					digPath:    []string{"build_gate", "heal"},
					wantFields: map[string]any{"retries": 2.0, "image": "docker.io/test/healer:latest", "command": "heal.sh"},
					wantAbsent: []string{"retain_container"},
				},
				{
					digPath:    []string{"build_gate", "heal", "envs"},
					wantFields: map[string]any{"HEALING_MODE": "auto"},
				},
			},
		},
		{
			name: "build_gate stack pre and post",
			ext:  ".yaml",
			spec: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  enabled: true
  pre:
    stack:
      enabled: true
      language: java
      release: 11
      default: true
  post:
    stack:
      enabled: true
      language: java
      release: "17"
      default: true
`,
			wantStepImage: "docker.io/test/mig:latest",
			digChecks: []digCheck{
				{
					digPath:    []string{"build_gate", "pre", "stack"},
					wantFields: map[string]any{"language": "java", "default": true, "release": 11.0},
				},
				{
					digPath:    []string{"build_gate", "post", "stack"},
					wantFields: map[string]any{"release": "17"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := runBuildSpecPayload(t, tt.spec, tt.ext, specPayloadOpts{})
			steps := mustSteps(t, result, 1)
			assertField(t, steps[0], "image", tt.wantStepImage)
			assertAbsent(t, steps[0], "retain_container")

			if tt.wantEnv != nil {
				envs := mustDig(t, result, "envs")
				for k, v := range tt.wantEnv {
					assertField(t, envs, k, v)
				}
			}
			if tt.wantRetries != 0 {
				heal := mustDig(t, result, "build_gate", "heal")
				assertField(t, heal, "retries", tt.wantRetries)
			}
			if tt.wantDomain != "" {
				assertField(t, result, "gitlab_domain", tt.wantDomain)
			}
			if tt.wantMRSuccess {
				assertField(t, result, "mr_on_success", true)
			}
			for _, dc := range tt.digChecks {
				target := mustDig(t, result, dc.digPath...)
				for k, v := range dc.wantFields {
					assertField(t, target, k, v)
				}
				for _, k := range dc.wantAbsent {
					assertAbsent(t, target, k)
				}
			}
		})
	}
}

func TestBuildSpecPayload_IncludeMerge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		fragment         string
		spec             string   // %s placeholder for fragment path
		digPath          []string // path to the merged object in the result
		useNormalize     bool     // call normalizeMigsSpecToJSON instead
		wantFields       map[string]any
		wantEnv          map[string]any // nil value → just check key existence
		wantArtifactsLen int
	}{
		{
			name: "heal include merge with inline overrides",
			fragment: `
retries: 2
image: docker.io/test/healer:latest
envs:
  A: from-fragment
  B: from-fragment
expectations:
  artifacts:
    - path: /out/gate-profile-candidate.json
      schema: gate_profile_v1
`,
			spec: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  heal:
    <<: !include %s
    retries: 1
    envs:
      B: inline-override
      C: inline-only
`,
			digPath:          []string{"build_gate", "heal"},
			wantFields:       map[string]any{"retries": 1.0, "image": "docker.io/test/healer:latest"},
			wantEnv:          map[string]any{"A": "from-fragment", "B": "inline-override", "C": "inline-only"},
			wantArtifactsLen: 1,
		},
		{
			name: "normalizeMigsSpecToJSON heal include merge",
			fragment: `
retries: 2
image: docker.io/test/healer:latest
envs:
  FRAGMENT_ONLY: yes
`,
			spec: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  heal:
    <<: !include %s
    envs:
      INLINE_ONLY: "true"
`,
			digPath:      []string{"build_gate", "heal"},
			useNormalize: true,
			wantEnv:      map[string]any{"FRAGMENT_ONLY": nil, "INLINE_ONLY": nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			fragPath := filepath.Join(tmpDir, "fragment.yaml")
			writeFile(t, fragPath, tt.fragment)
			specContent := fmt.Sprintf(tt.spec, fragPath)

			var result map[string]any
			if tt.useNormalize {
				normalized, err := normalizeMigsSpecToJSON(context.Background(), nil, nil, []byte(specContent), "")
				if err != nil {
					t.Fatalf("normalizeMigsSpecToJSON: %v", err)
				}
				result = unmarshalPayload(t, normalized)
			} else {
				result = buildAndParseSpec(t, tmpDir, specContent, ".yaml", specPayloadOpts{})
			}

			target := mustDig(t, result, tt.digPath...)
			for key, want := range tt.wantFields {
				assertField(t, target, key, want)
			}
			if tt.wantEnv != nil {
				envs := mustDig(t, target, "envs")
				for key, want := range tt.wantEnv {
					if want == nil {
						if _, ok := envs[key]; !ok {
							t.Errorf("expected envs.%s to exist", key)
						}
					} else {
						assertField(t, envs, key, want)
					}
				}
			}
			if tt.wantArtifactsLen > 0 {
				expectations := mustDig(t, target, "expectations")
				artifacts, ok := expectations["artifacts"].([]any)
				if !ok || len(artifacts) != tt.wantArtifactsLen {
					t.Fatalf("expected %d artifacts, got %v", tt.wantArtifactsLen, expectations["artifacts"])
				}
			}
		})
	}
}

func TestBuildSpecPayload_IncludePointerSelection(t *testing.T) {
	tests := []struct {
		name         string
		fragmentFile string
		fragment     string
		spec         string
		digPath      []string
		wantFields   map[string]any
		wantEnv      map[string]any
	}{
		{
			name:         "heal pointer include",
			fragmentFile: "heal-fragment.yaml",
			fragment:     "fragments:\n  heal:\n    image: docker.io/test/healer:latest\n    retries: 2\n",
			spec: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  heal:
    <<: !include ./heal-fragment.yaml#/fragments/heal
    retries: 1
`,
			digPath:    []string{"build_gate", "heal"},
			wantFields: map[string]any{"image": "docker.io/test/healer:latest", "retries": 1.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			writeFile(t, filepath.Join(tmpDir, tt.fragmentFile), tt.fragment)
			result := buildAndParseSpec(t, tmpDir, tt.spec, ".yaml", specPayloadOpts{})
			target := mustDig(t, result, tt.digPath...)
			for key, want := range tt.wantFields {
				assertField(t, target, key, want)
			}
			if tt.wantEnv != nil {
				envs := mustDig(t, target, "envs")
				for key, want := range tt.wantEnv {
					assertField(t, envs, key, want)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Individual tests (unique assertion patterns, cleaned up with helpers)
// ---------------------------------------------------------------------------

func TestBuildSpecPayload_MultiStepMigs(t *testing.T) {
	t.Parallel()
	result := runBuildSpecPayload(t, `
apiVersion: ploy.mig/v1alpha1
kind: MigRunSpec
steps:
  - image: docker.io/test/mig-step1:latest
    envs:
      STEP: "1"
      TARGET: java8
  - image: docker.io/test/mig-step2:latest
    envs:
      STEP: "2"
      TARGET: java11
  - image: docker.io/test/mig-step3:latest
    envs:
      STEP: "3"
      TARGET: java17
build_gate:
  enabled: true
  heal:
    retries: 1
    image: docker.io/test/healer:latest
`, ".yaml", specPayloadOpts{})

	steps := mustSteps(t, result, 3)
	assertField(t, steps[0], "image", "docker.io/test/mig-step1:latest")
	envs0 := mustDig(t, steps[0], "envs")
	assertField(t, envs0, "STEP", "1")

	assertField(t, steps[1], "image", "docker.io/test/mig-step2:latest")
	assertField(t, steps[2], "image", "docker.io/test/mig-step3:latest")
	mustDig(t, result, "build_gate", "heal")
}

func TestBuildSpecPayload_RelativePathsResolveFromSpecDir(t *testing.T) {
	specDir := t.TempDir()
	t.Chdir(t.TempDir())

	writeFile(t, filepath.Join(specDir, "heal-fragment.yaml"), "image: docker.io/test/healer-fragment:latest\n")

	result := buildAndParseSpec(t, specDir, `
envs:
  TOKEN: spec-dir-token
steps:
  - image: docker.io/test/mig:latest
build_gate:
  heal:
    <<: !include heal-fragment.yaml
`, ".yaml", specPayloadOpts{})

	envs := mustDig(t, result, "envs")
	assertField(t, envs, "TOKEN", "spec-dir-token")

	heal := mustDig(t, result, "build_gate", "heal")
	assertField(t, heal, "image", "docker.io/test/healer-fragment:latest")
}

func TestBuildSpecPayload_IncludeCycleDetected(t *testing.T) {
	specDir := t.TempDir()
	writeFile(t, filepath.Join(specDir, "a.yaml"), "heal: !include ./b.yaml#/heal\n")
	writeFile(t, filepath.Join(specDir, "b.yaml"), "heal: !include ./a.yaml#/heal\n")

	specPath := filepath.Join(specDir, "spec.yaml")
	writeFile(t, specPath, `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  heal: !include ./a.yaml#/heal
`)

	_, err := callBuildSpecPayload(t, specPath, specPayloadOpts{})
	if err == nil {
		t.Fatal("expected include cycle error")
	}
	if !strings.Contains(err.Error(), "spec include cycle detected") {
		t.Fatalf("error = %q, want include cycle error", err)
	}
}

func TestBuildSpecPayload_InPathInIncludedFragment(t *testing.T) {
	specDir := t.TempDir()
	fragmentsDir := filepath.Join(specDir, "fragments")
	if err := os.MkdirAll(fragmentsDir, 0o755); err != nil {
		t.Fatalf("mkdir fragments: %v", err)
	}

	writeFile(t, filepath.Join(fragmentsDir, "step.fragment.yaml"), `
step:
  image: docker.io/test/mig:latest
  in:
    - 0123456789ab:/in/input.txt
`)

	result := buildAndParseSpec(t, specDir, `
steps:
  - <<: !include ./fragments/step.fragment.yaml#/step
`, ".yaml", specPayloadOpts{})

	steps := mustSteps(t, result, 1)
	inRaw, ok := steps[0]["in"].([]any)
	if !ok || len(inRaw) != 1 {
		t.Fatalf("expected one in entry, got %v", steps[0]["in"])
	}
	inEntry, _ := inRaw[0].(string)
	if inEntry != "0123456789ab:/in/input.txt" {
		t.Fatalf("expected canonical in entry to stay unchanged, got %q", inEntry)
	}
}

func TestBuildSpecPayload_CLIOverrides(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		spec           string
		opts           specPayloadOpts
		wantStepCount  int
		wantStepImages []string
		wantStepEnvs   []map[string]any // per-step envs; nil entry = skip
		wantTopEnv     map[string]any
		wantTopFields  map[string]any
		wantTopAbsent  []string
		wantStepAbsent []string // checked on all steps
	}{
		{
			name: "full CLI overrides on spec with envs and gitlab",
			spec: `
steps:
  - image: docker.io/test/mig:v1
envs:
  KEY1: from_spec
  KEY2: value2
gitlab_domain: gitlab.com
`,
			opts: specPayloadOpts{
				migEnvs:      []string{"KEY1=from_cli", "KEY3=new_value"},
				migImage:     "docker.io/test/mig:v2",
				retain:       true,
				gitlabPAT:    "glpat-test",
				gitlabDomain: "gitlab.example.com",
				mrSuccess:    true,
			},
			wantStepCount:  1,
			wantStepImages: []string{"docker.io/test/mig:v2"},
			wantStepAbsent: []string{"retain_container"},
			wantTopEnv:     map[string]any{"KEY1": "from_cli", "KEY2": "value2", "KEY3": "new_value"},
			wantTopFields:  map[string]any{"gitlab_domain": "gitlab.example.com", "gitlab_pat": "glpat-test", "mr_on_success": true},
		},
		{
			name: "single-step image and env overrides",
			spec: `
steps:
  - image: docker.io/test/base:v1
envs:
  BASE_KEY: base_value
`,
			opts:           specPayloadOpts{migEnvs: []string{"CLI_KEY=cli_value"}, migImage: "docker.io/test/override:v2", retain: true},
			wantStepCount:  1,
			wantStepImages: []string{"docker.io/test/override:v2"},
			wantStepAbsent: []string{"retain_container"},
			wantTopEnv:     map[string]any{"BASE_KEY": "base_value", "CLI_KEY": "cli_value"},
			wantTopAbsent:  []string{"migs"},
		},
		{
			name: "multi-step ignores image/command CLI overrides",
			spec: `
steps:
  - image: docker.io/test/step1:v1
    envs:
      STEP: "1"
  - image: docker.io/test/step2:v1
    envs:
      STEP: "2"
`,
			opts:           specPayloadOpts{migEnvs: []string{"CLI_KEY=cli_value"}, migImage: "docker.io/test/override:v2", retain: true},
			wantStepCount:  2,
			wantStepImages: []string{"docker.io/test/step1:v1", "docker.io/test/step2:v1"},
			wantStepEnvs:   []map[string]any{{"STEP": "1"}, {"STEP": "2"}},
			wantTopEnv:     map[string]any{"CLI_KEY": "cli_value"},
			wantTopAbsent:  []string{"image", "retain_container"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := runBuildSpecPayload(t, tt.spec, ".yaml", tt.opts)
			steps := mustSteps(t, result, tt.wantStepCount)

			for i, wantImg := range tt.wantStepImages {
				assertField(t, steps[i], "image", wantImg)
				for _, key := range tt.wantStepAbsent {
					assertAbsent(t, steps[i], key)
				}
			}
			if tt.wantStepEnvs != nil {
				for i, wantEnv := range tt.wantStepEnvs {
					if wantEnv == nil {
						continue
					}
					envs := mustDig(t, steps[i], "envs")
					for k, v := range wantEnv {
						assertField(t, envs, k, v)
					}
				}
			}
			if tt.wantTopEnv != nil {
				envs := mustDig(t, result, "envs")
				for k, v := range tt.wantTopEnv {
					assertField(t, envs, k, v)
				}
			}
			for k, v := range tt.wantTopFields {
				assertField(t, result, k, v)
			}
			for _, key := range tt.wantTopAbsent {
				assertAbsent(t, result, key)
			}
		})
	}
}

func TestBuildSpecPayload_ImageInterpolationAcrossSections(t *testing.T) {
	t.Setenv("PLOY_TEST_IMG", "docker.io/test/amata:latest")
	t.Setenv("PLOY_TEST_STEP_DEFAULT", "docker.io/test/default-step:latest")
	t.Setenv("PLOY_TEST_STEP_MAVEN", "docker.io/test/maven-step:latest")

	result := runBuildSpecPayload(t, `
steps:
  - image:
      default: $PLOY_TEST_STEP_DEFAULT
      java-maven: ${PLOY_TEST_STEP_MAVEN}
build_gate:
  heal:
    retries: 1
    image: ${PLOY_TEST_IMG}
`, ".yaml", specPayloadOpts{})

	steps := mustSteps(t, result, 1)
	stepImage := mustDig(t, steps[0], "image")
	assertField(t, stepImage, "default", "docker.io/test/default-step:latest")
	assertField(t, stepImage, "java-maven", "docker.io/test/maven-step:latest")

	heal := mustDig(t, result, "build_gate", "heal")
	assertField(t, heal, "image", "docker.io/test/amata:latest")
}
