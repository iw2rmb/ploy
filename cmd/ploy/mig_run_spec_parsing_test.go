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
			name: "healing spec_path invalid type",
			spec: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  healing:
    by_error_kind:
      infra:
        spec_path: 123
  router:
    image: docker.io/test/router:latest
`,
		},
		{
			name: "router spec_path invalid type",
			spec: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  router:
    spec_path: 123
`,
		},
		{
			name: "amata spec_path missing file",
			spec: `
steps:
  - image: docker.io/test/step1:latest
    amata:
      spec: $PLOY_PATH/missing-amata.yaml
`,
			setenv: map[string]string{"PLOY_PATH": ""},
		},
		{
			name: "step retain_container forbidden",
			spec: `
steps:
  - image: docker.io/test/mig:latest
    retain_container: true
`,
			wantErr: "validate spec: steps[0].retain_container: forbidden",
		},
		{
			name: "healing retain_container forbidden",
			spec: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  healing:
    by_error_kind:
      infra:
        image: docker.io/test/heal:latest
        retain_container: true
  router:
    image: docker.io/test/router:latest
`,
			wantErr: "validate spec: build_gate.healing.by_error_kind.infra.retain_container: forbidden",
		},
		{
			name: "router retain_container forbidden",
			spec: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  router:
    image: docker.io/test/router:latest
    retain_container: true
`,
			wantErr: "validate spec: build_gate.router.retain_container: forbidden",
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
			cmd := steps[0]["command"]

			if tt.wantCmdArr != nil {
				arr, ok := cmd.([]any)
				if !ok {
					t.Fatalf("expected command as []any, got %T", cmd)
				}
				if len(arr) != len(tt.wantCmdArr) {
					t.Fatalf("command len = %d, want %d: %v", len(arr), len(tt.wantCmdArr), arr)
				}
				for i, want := range tt.wantCmdArr {
					if got, _ := arr[i].(string); got != want {
						t.Errorf("command[%d] = %q, want %q", i, got, want)
					}
				}
			}
			if tt.wantCmdStr != "" {
				got, ok := cmd.(string)
				if !ok || got != tt.wantCmdStr {
					t.Errorf("command = %v, want %q", cmd, tt.wantCmdStr)
				}
			}
		})
	}
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
	}{
		{
			name: "YAML spec",
			ext:  ".yaml",
			spec: `
steps:
  - image: docker.io/test/mig:latest
env:
  KEY1: value1
  KEY2: value2
build_gate:
  healing:
    by_error_kind:
      infra:
        retries: 1
        image: docker.io/test/healer:latest
  router:
    image: docker.io/test/router:latest
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
  "env": {"KEY1": "value1"},
  "build_gate": {
    "healing": {
      "by_error_kind": {
        "infra": {
          "retries": 2,
          "image": "docker.io/test/healer:latest"
        }
      }
    },
    "router": {"image": "docker.io/test/router:latest"}
  }
}`,
			wantStepImage: "docker.io/test/mig:latest",
			wantRetries:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := runBuildSpecPayload(t, tt.spec, tt.ext, specPayloadOpts{})
			steps := mustSteps(t, result, 1)
			assertField(t, steps[0], "image", tt.wantStepImage)
			assertAbsent(t, steps[0], "retain_container")
			mustDig(t, result, "build_gate", "healing")

			if tt.wantEnv != nil {
				env := mustDig(t, result, "env")
				for k, v := range tt.wantEnv {
					assertField(t, env, k, v)
				}
			}
			if tt.wantRetries != 0 {
				infra := mustDig(t, result, "build_gate", "healing", "by_error_kind", "infra")
				assertField(t, infra, "retries", tt.wantRetries)
			}
			if tt.wantDomain != "" {
				assertField(t, result, "gitlab_domain", tt.wantDomain)
			}
			if tt.wantMRSuccess {
				assertField(t, result, "mr_on_success", true)
			}
		})
	}
}

func TestBuildSpecPayload_SpecPathMerge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		fragment         string
		spec             string   // %s placeholder for fragment path
		digPath          []string // path to the merged object in the result
		useNormalize     bool     // call normalizeMigsSpecToJSON instead
		wantFields       map[string]any
		wantEnv          map[string]any // nil value → just check key existence
		wantAbsent       []string
		wantArtifactsLen int
	}{
		{
			name: "healing fragment merge with inline overrides",
			fragment: `
retries: 2
image: docker.io/test/healer:latest
env:
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
  healing:
    by_error_kind:
      infra:
        spec_path: %s
        retries: 1
        env:
          B: inline-override
          C: inline-only
  router:
    image: docker.io/test/router:latest
`,
			digPath:          []string{"build_gate", "healing", "by_error_kind", "infra"},
			wantFields:       map[string]any{"retries": 1.0, "image": "docker.io/test/healer:latest"},
			wantEnv:          map[string]any{"A": "from-fragment", "B": "inline-override", "C": "inline-only"},
			wantAbsent:       []string{"spec_path"},
			wantArtifactsLen: 1,
		},
		{
			name: "router fragment merge with inline overrides",
			fragment: `
image: docker.io/test/router:latest
env:
  A: from-fragment
  B: from-fragment
`,
			spec: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  router:
    spec_path: %s
    env:
      B: inline-override
      C: inline-only
`,
			digPath:    []string{"build_gate", "router"},
			wantFields: map[string]any{"image": "docker.io/test/router:latest"},
			wantEnv:    map[string]any{"A": "from-fragment", "B": "inline-override", "C": "inline-only"},
			wantAbsent: []string{"spec_path"},
		},
		{
			name: "normalizeMigsSpecToJSON healing fragment merge",
			fragment: `
retries: 2
image: docker.io/test/healer:latest
env:
  FRAGMENT_ONLY: yes
`,
			spec: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  healing:
    by_error_kind:
      infra:
        spec_path: %s
        env:
          INLINE_ONLY: "true"
  router:
    image: docker.io/test/router:latest
`,
			digPath:      []string{"build_gate", "healing", "by_error_kind", "infra"},
			useNormalize: true,
			wantEnv:      map[string]any{"FRAGMENT_ONLY": nil, "INLINE_ONLY": nil},
			wantAbsent:   []string{"spec_path"},
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
				normalized, err := normalizeMigsSpecToJSON(context.Background(), nil, nil, []byte(specContent))
				if err != nil {
					t.Fatalf("normalizeMigsSpecToJSON: %v", err)
				}
				result = unmarshalPayload(t, normalized)
			} else {
				result = buildAndParseSpec(t, tmpDir, specContent, ".yaml", specPayloadOpts{})
			}

			target := mustDig(t, result, tt.digPath...)
			for _, key := range tt.wantAbsent {
				assertAbsent(t, target, key)
			}
			for key, want := range tt.wantFields {
				assertField(t, target, key, want)
			}
			if tt.wantEnv != nil {
				env := mustDig(t, target, "env")
				for key, want := range tt.wantEnv {
					if want == nil {
						if _, ok := env[key]; !ok {
							t.Errorf("expected env.%s to exist", key)
						}
					} else {
						assertField(t, env, key, want)
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

func TestBuildSpecPayload_SpecPathEnvExpansion(t *testing.T) {
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
			name:         "healing $PLOY_PATH expansion",
			fragmentFile: "infra-fragment.yaml",
			fragment:     "image: docker.io/test/healer:latest\nretries: 2\n",
			spec: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  healing:
    by_error_kind:
      infra:
        spec_path: $PLOY_PATH/infra-fragment.yaml
        retries: 1
  router:
    image: docker.io/test/router:latest
`,
			digPath:    []string{"build_gate", "healing", "by_error_kind", "infra"},
			wantFields: map[string]any{"image": "docker.io/test/healer:latest", "retries": 1.0},
		},
		{
			name:         "router ${PLOY_PATH} expansion",
			fragmentFile: "router-fragment.yaml",
			fragment:     "image: docker.io/test/router:latest\nenv:\n  A: from-fragment\n",
			spec: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  router:
    spec_path: ${PLOY_PATH}/router-fragment.yaml
    env:
      B: inline-only
`,
			digPath:    []string{"build_gate", "router"},
			wantFields: map[string]any{"image": "docker.io/test/router:latest"},
			wantEnv:    map[string]any{"A": "from-fragment", "B": "inline-only"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			writeFile(t, filepath.Join(tmpDir, tt.fragmentFile), tt.fragment)
			t.Setenv("PLOY_PATH", tmpDir)

			result := buildAndParseSpec(t, tmpDir, tt.spec, ".yaml", specPayloadOpts{})
			target := mustDig(t, result, tt.digPath...)

			assertAbsent(t, target, "spec_path")
			for key, want := range tt.wantFields {
				assertField(t, target, key, want)
			}
			if tt.wantEnv != nil {
				env := mustDig(t, target, "env")
				for key, want := range tt.wantEnv {
					assertField(t, env, key, want)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Individual tests (unique assertion patterns, cleaned up with helpers)
// ---------------------------------------------------------------------------

func TestBuildSpecPayload_BuildGateStackPrePost(t *testing.T) {
	t.Parallel()
	result := runBuildSpecPayload(t, `
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
`, ".yaml", specPayloadOpts{})

	preStack := mustDig(t, result, "build_gate", "pre", "stack")
	assertField(t, preStack, "language", "java")
	assertField(t, preStack, "default", true)
	assertField(t, preStack, "release", 11.0)

	postStack := mustDig(t, result, "build_gate", "post", "stack")
	assertField(t, postStack, "release", "17")
}

func TestBuildSpecPayload_ContainsBuildGateHealing(t *testing.T) {
	t.Parallel()
	result := runBuildSpecPayload(t, `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  healing:
    by_error_kind:
      infra:
        retries: 2
        image: docker.io/test/healer:latest
        command: "heal.sh"
        env:
          HEALING_MODE: auto
  router:
    image: docker.io/test/router:latest
`, ".yaml", specPayloadOpts{})

	infra := mustDig(t, result, "build_gate", "healing", "by_error_kind", "infra")
	assertField(t, infra, "retries", 2.0)
	assertField(t, infra, "image", "docker.io/test/healer:latest")
	assertField(t, infra, "command", "heal.sh")
	assertAbsent(t, infra, "retain_container")

	env := mustDig(t, infra, "env")
	assertField(t, env, "HEALING_MODE", "auto")
}

func TestBuildSpecPayload_MultiStepMigs(t *testing.T) {
	t.Parallel()
	result := runBuildSpecPayload(t, `
apiVersion: ploy.mig/v1alpha1
kind: MigRunSpec
steps:
  - image: docker.io/test/mig-step1:latest
    env:
      STEP: "1"
      TARGET: java8
  - image: docker.io/test/mig-step2:latest
    env:
      STEP: "2"
      TARGET: java11
  - image: docker.io/test/mig-step3:latest
    env:
      STEP: "3"
      TARGET: java17
build_gate:
  enabled: true
  healing:
    by_error_kind:
      infra:
        retries: 1
        image: docker.io/test/healer:latest
  router:
    image: docker.io/test/router:latest
`, ".yaml", specPayloadOpts{})

	steps := mustSteps(t, result, 3)
	assertField(t, steps[0], "image", "docker.io/test/mig-step1:latest")
	env0 := mustDig(t, steps[0], "env")
	assertField(t, env0, "STEP", "1")

	assertField(t, steps[1], "image", "docker.io/test/mig-step2:latest")
	assertField(t, steps[2], "image", "docker.io/test/mig-step3:latest")
	mustDig(t, result, "build_gate", "healing")
}

func TestBuildSpecPayload_MultiStepMigsWithEnvFromFile(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	envFile1 := filepath.Join(tmpDir, "env1.txt")
	envFile2 := filepath.Join(tmpDir, "env2.txt")
	writeFile(t, envFile1, "secret-token-1")
	writeFile(t, envFile2, "secret-token-2")

	result := buildAndParseSpec(t, tmpDir, fmt.Sprintf(`
steps:
  - image: docker.io/test/mig1:latest
    env_from_file:
      TOKEN: %s
  - image: docker.io/test/mig2:latest
    env_from_file:
      TOKEN: %s
`, envFile1, envFile2), ".yaml", specPayloadOpts{})

	steps := mustSteps(t, result, 2)
	env0 := mustDig(t, steps[0], "env")
	assertField(t, env0, "TOKEN", "secret-token-1")
	env1 := mustDig(t, steps[1], "env")
	assertField(t, env1, "TOKEN", "secret-token-2")
	assertAbsent(t, steps[0], "env_from_file")
	assertAbsent(t, steps[1], "env_from_file")
}

func TestBuildSpecPayload_RelativePathsResolveFromSpecDir(t *testing.T) {
	specDir := t.TempDir()
	t.Chdir(t.TempDir())

	writeFile(t, filepath.Join(specDir, "secret.txt"), "secret-from-spec-dir")
	writeFile(t, filepath.Join(specDir, "router-fragment.yaml"), "image: docker.io/test/router-fragment:latest\n")
	writeFile(t, filepath.Join(specDir, "infra-fragment.yaml"), "image: docker.io/test/healer-fragment:latest\n")
	amataContent := "version: amata/v1\nname: rel-path\n"
	writeFile(t, filepath.Join(specDir, "amata.yaml"), amataContent)

	result := buildAndParseSpec(t, specDir, `
env_from_file:
  TOKEN: secret.txt
steps:
  - image: docker.io/test/mig:latest
    amata:
      spec: amata.yaml
build_gate:
  healing:
    by_error_kind:
      infra:
        spec_path: infra-fragment.yaml
  router:
    spec_path: router-fragment.yaml
`, ".yaml", specPayloadOpts{})

	env := mustDig(t, result, "env")
	assertField(t, env, "TOKEN", "secret-from-spec-dir")

	steps := mustSteps(t, result, 1)
	amata := mustDig(t, steps[0], "amata")
	assertField(t, amata, "spec", amataContent)

	infra := mustDig(t, result, "build_gate", "healing", "by_error_kind", "infra")
	assertField(t, infra, "image", "docker.io/test/healer-fragment:latest")

	router := mustDig(t, result, "build_gate", "router")
	assertField(t, router, "image", "docker.io/test/router-fragment:latest")
}

func TestBuildSpecPayload_CanonicalSingleStepWithOverrides(t *testing.T) {
	t.Parallel()
	result := runBuildSpecPayload(t, `
steps:
  - image: docker.io/test/base:v1
env:
  BASE_KEY: base_value
`, ".yaml", specPayloadOpts{
		migEnvs:  []string{"CLI_KEY=cli_value"},
		migImage: "docker.io/test/override:v2",
		retain:   true,
	})

	steps := mustSteps(t, result, 1)
	assertField(t, steps[0], "image", "docker.io/test/override:v2")
	assertAbsent(t, steps[0], "retain_container")

	env := mustDig(t, result, "env")
	assertField(t, env, "BASE_KEY", "base_value")
	assertField(t, env, "CLI_KEY", "cli_value")
	assertAbsent(t, result, "migs")
}

func TestBuildSpecPayload_MultiStepIgnoresCLIOverrides(t *testing.T) {
	t.Parallel()
	result := runBuildSpecPayload(t, `
steps:
  - image: docker.io/test/step1:v1
    env:
      STEP: "1"
  - image: docker.io/test/step2:v1
    env:
      STEP: "2"
`, ".yaml", specPayloadOpts{
		migEnvs:  []string{"CLI_KEY=cli_value"},
		migImage: "docker.io/test/override:v2",
		retain:   true,
	})

	steps := mustSteps(t, result, 2)

	// First step unchanged (CLI overrides not applied to steps).
	assertField(t, steps[0], "image", "docker.io/test/step1:v1")
	env0 := mustDig(t, steps[0], "env")
	if len(env0) != 1 {
		t.Errorf("expected steps[0].env to have 1 key, got %d: %v", len(env0), env0)
	}
	assertField(t, env0, "STEP", "1")

	assertField(t, steps[1], "image", "docker.io/test/step2:v1")

	// Top-level env override applied.
	topEnv := mustDig(t, result, "env")
	assertField(t, topEnv, "CLI_KEY", "cli_value")

	// Image/retain overrides not applied at top level.
	assertAbsent(t, result, "image")
	assertAbsent(t, result, "retain_container")
}

func TestBuildSpecPayload_StepAmataSpecPathResolved(t *testing.T) {
	tmpDir := t.TempDir()
	amataContent := "version: amata/v1\nname: codex-step\n"
	writeFile(t, filepath.Join(tmpDir, "amata.yaml"), amataContent)
	t.Setenv("PLOY_PATH", tmpDir)

	result := buildAndParseSpec(t, tmpDir, `
steps:
  - image: docker.io/test/step1:latest
  - image: docker.io/test/step2:latest
    amata:
      spec: $PLOY_PATH/amata.yaml
      set:
        - param: model
          value: gpt-5
`, ".yaml", specPayloadOpts{})

	steps := mustSteps(t, result, 2)
	amata := mustDig(t, steps[1], "amata")
	assertField(t, amata, "spec", amataContent)

	set, ok := amata["set"].([]any)
	if !ok || len(set) != 1 {
		t.Fatalf("expected amata.set len=1, got %v", amata["set"])
	}
	param := set[0].(map[string]any)
	assertField(t, param, "param", "model")
	assertField(t, param, "value", "gpt-5")
}

func TestBuildSpecPayload_ImageInterpolationAcrossSections(t *testing.T) {
	t.Setenv("PLOY_TEST_IMG", "docker.io/test/codex:latest")
	t.Setenv("PLOY_TEST_STEP_DEFAULT", "docker.io/test/default-step:latest")
	t.Setenv("PLOY_TEST_STEP_MAVEN", "docker.io/test/maven-step:latest")

	result := runBuildSpecPayload(t, `
steps:
  - image:
      default: $PLOY_TEST_STEP_DEFAULT
      java-maven: ${PLOY_TEST_STEP_MAVEN}
build_gate:
  router:
    image: $PLOY_TEST_IMG
  healing:
    by_error_kind:
      infra:
        retries: 1
        image: ${PLOY_TEST_IMG}
`, ".yaml", specPayloadOpts{})

	steps := mustSteps(t, result, 1)
	stepImage := mustDig(t, steps[0], "image")
	assertField(t, stepImage, "default", "docker.io/test/default-step:latest")
	assertField(t, stepImage, "java-maven", "docker.io/test/maven-step:latest")

	router := mustDig(t, result, "build_gate", "router")
	assertField(t, router, "image", "docker.io/test/codex:latest")

	infra := mustDig(t, result, "build_gate", "healing", "by_error_kind", "infra")
	assertField(t, infra, "image", "docker.io/test/codex:latest")
}

func TestBuildSpecPayload_ImageInterpolation_UnresolvedReturnsError(t *testing.T) {
	specFile := filepath.Join(t.TempDir(), "spec.yaml")
	writeFile(t, specFile, `
steps:
  - image: $PLOY_TEST_MISSING_IMAGE
`)

	_, err := callBuildSpecPayload(t, specFile, specPayloadOpts{})
	if err == nil {
		t.Fatalf("expected unresolved image placeholder error")
	}
	if !strings.Contains(err.Error(), `steps[0].image: unresolved environment variables: PLOY_TEST_MISSING_IMAGE`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
