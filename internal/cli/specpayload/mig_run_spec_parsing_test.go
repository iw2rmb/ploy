package specpayload

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// specPayloadOpts holds optional Build parameters.
// Zero values correspond to the default (nil/empty/false) arguments.
type specPayloadOpts struct {
	migEnvs    []string
	migImage   string
	retain     bool
	migCommand string
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", filepath.Base(path), err)
	}
}

func callBuildSpecPayload(t *testing.T, specFile string, opts specPayloadOpts) ([]byte, error) {
	t.Helper()
	return Build(
		context.Background(), nil, nil, specFile,
		opts.migEnvs, opts.migImage, opts.retain, opts.migCommand,
	)
}

// buildAndParseSpec writes specContent to dir/spec{ext}, calls Build,
// and returns the parsed JSON map. Use when the caller needs to write auxiliary
// files (fragments, env files) to the same directory before this call.
func buildAndParseSpec(t *testing.T, dir, specContent, ext string, opts specPayloadOpts) map[string]any {
	t.Helper()
	specFile := filepath.Join(dir, "spec"+ext)
	writeFile(t, specFile, specContent)
	payload, err := callBuildSpecPayload(t, specFile, opts)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return unmarshalPayload(t, payload)
}

// runBuildSpecPayload creates a temp dir (or passes empty specFile when
// specContent is empty), calls Build, and returns the parsed map.
func runBuildSpecPayload(t *testing.T, specContent, ext string, opts specPayloadOpts) map[string]any {
	t.Helper()
	if specContent == "" {
		payload, err := callBuildSpecPayload(t, "", opts)
		if err != nil {
			t.Fatalf("Build: %v", err)
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
		files    map[string]string
		wantErr  string // substring match; empty → just assert err != nil
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
		{
			name: "unresolved build gate image env placeholder",
			spec: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  images:
    - stack:
        language: java
        release: "17"
        tool: maven
      image: $PLOY_TEST_MISSING_GATE_IMAGE
`,
			wantErr: "resolve image env placeholders: build_gate.images[0].image: unresolved environment variables: PLOY_TEST_MISSING_GATE_IMAGE",
		},
		{
			name: "missing hydra input file",
			spec: `
steps:
  - image: docker.io/test/mig:latest
    in:
      - ./missing.yaml:missing.yaml
`,
			wantErr: "validate local file records: steps[0].in[0]: source",
		},
		{
			name: "amata include not mounted",
			spec: `
steps:
  - image: docker.io/test/mig:latest
    in:
      - ./amata.yaml:amata.yaml
      - ./gradle-classpath.yaml:gradle-classpath.yaml
`,
			files: map[string]string{
				"amata.yaml":            "flows:\n  main: !include ./gradle-assemble.yaml#/flows/gradle_assemble_audit\n",
				"gradle-classpath.yaml": "flows: {}\n",
				"gradle-assemble.yaml":  "flows:\n  gradle_assemble_audit:\n    steps: []\n",
			},
			wantErr: "validate local file records: steps[0].in include ./gradle-assemble.yaml#/flows/gradle_assemble_audit: target /in/gradle-assemble.yaml is not mounted by this step's in entries",
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
				for rel, content := range tt.files {
					writeFile(t, filepath.Join(tmpDir, rel), content)
				}
				specPath := filepath.Join(tmpDir, "spec.yaml")
				writeFile(t, specPath, tt.spec)
				_, err = callBuildSpecPayload(t, specPath, specPayloadOpts{})
			}

			if err == nil {
				t.Fatal("expected error")
			}
			if tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want containing %q", err.Error(), tt.wantErr)
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
		digChecks     []digCheck // optional nested field assertions
	}{
		{
			name: "YAML spec",
			ext:  ".yaml",
			spec: `
steps:
  - image: docker.io/test/mig:latest
    options:
      mount_docker_socket: true
envs:
  KEY1: value1
  KEY2: value2
`,
			wantStepImage: "docker.io/test/mig:latest",
			wantEnv:       map[string]any{"KEY1": "value1", "KEY2": "value2"},
		},
		{
			name: "JSON spec",
			ext:  ".json",
			spec: `{
  "steps": [{"image": "docker.io/test/mig:latest"}],
  "envs": {"KEY1": "value1"}
}`,
			wantStepImage: "docker.io/test/mig:latest",
		},
		{
			name: "build_gate stack pre and post",
			ext:  ".yaml",
			spec: `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  pre:
    stack:
      mode: fallback
      language: java
      tool: maven
      release: 11
  post:
    stack:
      mode: strict
      language: java
      tool: maven
      release: "17"
`,
			wantStepImage: "docker.io/test/mig:latest",
			digChecks: []digCheck{
				{
					digPath:    []string{"build_gate", "pre", "stack"},
					wantFields: map[string]any{"mode": "fallback", "language": "java", "tool": "maven", "release": 11.0},
				},
				{
					digPath:    []string{"build_gate", "post", "stack"},
					wantFields: map[string]any{"mode": "strict", "release": "17"},
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
			if optionsRaw, ok := steps[0]["options"]; ok {
				options, ok := optionsRaw.(map[string]any)
				if !ok {
					t.Fatalf("steps[0].options = %T, want object", optionsRaw)
				}
				assertField(t, options, "mount_docker_socket", true)
			}

			if tt.wantEnv != nil {
				envs := mustDig(t, result, "envs")
				for k, v := range tt.wantEnv {
					assertField(t, envs, k, v)
				}
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

func TestBuildSpecPayload_BuildGateImageTemplateExpansion(t *testing.T) {
	t.Parallel()

	spec := `
steps:
  - image: docker.io/test/mig:latest
build_gate:
  images:
    - stack:
        language: java
        release: "17"
        tool: maven
      image: docker.io/test/mig-${stack.language}-${stack.release}-${stack.tool}:latest
`

	result := runBuildSpecPayload(t, spec, ".yaml", specPayloadOpts{})
	buildGate := mustDig(t, result, "build_gate")
	rawImages, ok := buildGate["images"].([]any)
	if !ok || len(rawImages) != 1 {
		t.Fatalf("build_gate.images = %T (%v), want len=1 array", buildGate["images"], buildGate["images"])
	}
	rule, ok := rawImages[0].(map[string]any)
	if !ok {
		t.Fatalf("build_gate.images[0] = %T, want object", rawImages[0])
	}
	assertField(t, rule, "image", "docker.io/test/mig-java-17-maven:latest")
}

func TestBuildSpecPayload_IncludePointerSelection(t *testing.T) {
	tests := []struct {
		name         string
		fragmentFile string
		fragment     string
		spec         string
		wantImage    string
		wantEnv      map[string]any
	}{
		{
			name:         "step pointer include",
			fragmentFile: "step-fragment.yaml",
			fragment:     "fragments:\n  step:\n    image: docker.io/test/from-fragment:latest\n    envs:\n      A: from-fragment\n      B: from-fragment\n",
			spec: `
steps:
  - <<: !include ./step-fragment.yaml#/fragments/step
    envs:
      B: inline
      C: inline
`,
			wantImage: "docker.io/test/from-fragment:latest",
			wantEnv:   map[string]any{"A": "from-fragment", "B": "inline", "C": "inline"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			writeFile(t, filepath.Join(tmpDir, tt.fragmentFile), tt.fragment)
			result := buildAndParseSpec(t, tmpDir, tt.spec, ".yaml", specPayloadOpts{})
			steps := mustSteps(t, result, 1)
			assertField(t, steps[0], "image", tt.wantImage)
			if tt.wantEnv != nil {
				envs := mustDig(t, steps[0], "envs")
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
name: multi-step-migs
description: Multi-step parsing fixture.
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
  disabled: false
`, ".yaml", specPayloadOpts{})

	steps := mustSteps(t, result, 3)
	assertField(t, steps[0], "image", "docker.io/test/mig-step1:latest")
	envs0 := mustDig(t, steps[0], "envs")
	assertField(t, envs0, "STEP", "1")

	assertField(t, steps[1], "image", "docker.io/test/mig-step2:latest")
	assertField(t, steps[2], "image", "docker.io/test/mig-step3:latest")
}

func TestBuildSpecPayload_RefExpansion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(t *testing.T, rootDir string) string
		wantImage string
		wantEnv   map[string]any
	}{
		{
			name: "relative file ref imports selected step only",
			setup: func(t *testing.T, rootDir string) string {
				libDir := filepath.Join(rootDir, "lib")
				if err := os.MkdirAll(libDir, 0o755); err != nil {
					t.Fatalf("mkdir lib: %v", err)
				}
				writeFile(t, filepath.Join(libDir, "mig.yaml"), `
envs:
  IGNORED: top-level
build_gate:
  disabled: true
steps:
  - name: ignored
    image: docker.io/test/ignored:latest
  - name: reuse
    image: docker.io/test/reuse:latest
    envs:
      STEP_ENV: kept
`)
				return `
envs:
  ROOT_ENV: kept
build_gate:
  disabled: false
steps:
  - ref: ./lib/mig.yaml:reuse
`
			},
			wantImage: "docker.io/test/reuse:latest",
			wantEnv:   map[string]any{"STEP_ENV": "kept"},
		},
		{
			name: "ref wrapper envs overlay imported step envs",
			setup: func(t *testing.T, rootDir string) string {
				writeFile(t, filepath.Join(rootDir, "lib.yaml"), `
steps:
  - name: reuse
    image: docker.io/test/reuse:latest
    envs:
      IMPORTED_ONLY: imported
      SHARED: imported
`)
				return `
steps:
  - ref: ./lib.yaml:reuse
    envs:
      WRAPPER_ONLY: wrapper
      SHARED: wrapper
`
			},
			wantImage: "docker.io/test/reuse:latest",
			wantEnv: map[string]any{
				"IMPORTED_ONLY": "imported",
				"WRAPPER_ONLY":  "wrapper",
				"SHARED":        "wrapper",
			},
		},
		{
			name: "directory ref uses mig yaml",
			setup: func(t *testing.T, rootDir string) string {
				libDir := filepath.Join(rootDir, "reusable")
				if err := os.MkdirAll(libDir, 0o755); err != nil {
					t.Fatalf("mkdir reusable: %v", err)
				}
				writeFile(t, filepath.Join(libDir, "mig.yaml"), `
steps:
  - name: reuse
    image: docker.io/test/from-dir:latest
`)
				return `
steps:
  - ref: ./reusable:reuse
`
			},
			wantImage: "docker.io/test/from-dir:latest",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rootDir := t.TempDir()
			spec := tt.setup(t, rootDir)
			result := buildAndParseSpec(t, rootDir, spec, ".yaml", specPayloadOpts{})
			steps := mustSteps(t, result, 1)
			assertField(t, steps[0], "image", tt.wantImage)
			assertField(t, steps[0], "name", "reuse")
			if tt.wantEnv != nil {
				envs := mustDig(t, steps[0], "envs")
				for key, want := range tt.wantEnv {
					assertField(t, envs, key, want)
				}
			}
			if topEnvs, ok := result["envs"].(map[string]any); ok {
				if _, exists := topEnvs["IGNORED"]; exists {
					t.Fatalf("referenced top-level envs must not be imported: %#v", topEnvs)
				}
			}
			if _, ok := result["build_gate"]; ok {
				buildGate := mustDig(t, result, "build_gate")
				assertField(t, buildGate, "disabled", false)
			}
		})
	}
}

func TestBuildSpecPayload_RefExpansionErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		files   map[string]string
		spec    string
		wantErr string
	}{
		{
			name: "missing selector step name",
			spec: `
steps:
  - ref: "./lib.yaml:"
`,
			files:   map[string]string{"lib.yaml": "steps: []\n"},
			wantErr: "step name is required",
		},
		{
			name: "missing step names in referenced spec",
			spec: `
steps:
  - ref: ./lib.yaml:reuse
`,
			files:   map[string]string{"lib.yaml": "steps:\n  - image: docker.io/test/reuse:latest\n"},
			wantErr: "steps[0].name is required for step selection",
		},
		{
			name: "duplicate step names in referenced spec",
			spec: `
steps:
  - ref: ./lib.yaml:reuse
`,
			files:   map[string]string{"lib.yaml": "steps:\n  - name: reuse\n    image: docker.io/test/a:latest\n  - name: reuse\n    image: docker.io/test/b:latest\n"},
			wantErr: "duplicate",
		},
		{
			name: "missing referenced step",
			spec: `
steps:
  - ref: ./lib.yaml:missing
`,
			files:   map[string]string{"lib.yaml": "steps:\n  - name: reuse\n    image: docker.io/test/reuse:latest\n"},
			wantErr: "step \"missing\" not found",
		},
		{
			name: "ref step cannot mix non-env keys",
			spec: `
steps:
  - ref: ./lib.yaml:reuse
    image: docker.io/test/inline:latest
`,
			files:   map[string]string{"lib.yaml": "steps:\n  - name: reuse\n    image: docker.io/test/reuse:latest\n"},
			wantErr: "ref step may contain only ref and envs",
		},
		{
			name: "ref step envs must be object",
			spec: `
steps:
  - ref: ./lib.yaml:reuse
    envs: invalid
`,
			files:   map[string]string{"lib.yaml": "steps:\n  - name: reuse\n    image: docker.io/test/reuse:latest\n"},
			wantErr: "steps[0].envs: expected object",
		},
		{
			name: "cycle",
			spec: `
steps:
  - name: root
    image: docker.io/test/root:latest
  - ref: ./lib.yaml:reuse
`,
			files:   map[string]string{"lib.yaml": "steps:\n  - ref: ./spec.yaml:root\n  - name: reuse\n    image: docker.io/test/reuse:latest\n"},
			wantErr: "spec ref cycle detected",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tmpDir := t.TempDir()
			for rel, content := range tt.files {
				writeFile(t, filepath.Join(tmpDir, rel), content)
			}
			specPath := filepath.Join(tmpDir, "spec.yaml")
			writeFile(t, specPath, tt.spec)
			_, err := callBuildSpecPayload(t, specPath, specPayloadOpts{})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateLocal_RefExpansionNormalizesLocalPathsFromSourceSpec(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	libDir := filepath.Join(rootDir, "library")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatalf("mkdir library: %v", err)
	}
	writeFile(t, filepath.Join(libDir, "input.txt"), "hello\n")
	writeFile(t, filepath.Join(libDir, "auth.toml"), "auth\n")
	writeFile(t, filepath.Join(libDir, "tool.jar"), "tool\n")
	writeFile(t, filepath.Join(libDir, "mig.yaml"), `
steps:
  - name: with-files
    image: docker.io/test/reuse:latest
    in:
      - ./input.txt:input.txt
    home:
      - ./auth.toml:.codex/config.toml:ro
    tmp:
      - ./tool.jar:lib/tool.jar
`)
	specPath := filepath.Join(rootDir, "spec.yaml")
	writeFile(t, specPath, `
steps:
  - ref: ./library/mig.yaml:with-files
`)

	payload, err := ValidateLocalFile(specPath)
	if err != nil {
		t.Fatalf("ValidateLocalFile: %v", err)
	}
	result := unmarshalPayload(t, payload)
	steps := mustSteps(t, result, 1)
	inEntries, ok := steps[0]["in"].([]any)
	if !ok || len(inEntries) != 1 {
		t.Fatalf("steps[0].in = %#v, want one entry", steps[0]["in"])
	}
	entry, ok := inEntries[0].(string)
	if !ok || !shortHashPattern.MatchString(strings.Split(entry, ":")[0]) || !strings.HasSuffix(entry, ":/in/input.txt") {
		t.Fatalf("steps[0].in[0] = %q, want canonical input entry", entry)
	}
	homeEntries, ok := steps[0]["home"].([]any)
	if !ok || len(homeEntries) != 1 {
		t.Fatalf("steps[0].home = %#v, want one entry", steps[0]["home"])
	}
	homeEntry, ok := homeEntries[0].(string)
	if !ok || !shortHashPattern.MatchString(strings.Split(homeEntry, ":")[0]) || !strings.HasSuffix(homeEntry, ":.codex/config.toml:ro") {
		t.Fatalf("steps[0].home[0] = %q, want canonical home entry", homeEntry)
	}
	tmpEntries, ok := steps[0]["tmp"].([]any)
	if !ok || len(tmpEntries) != 1 {
		t.Fatalf("steps[0].tmp = %#v, want one entry", steps[0]["tmp"])
	}
	tmpEntry, ok := tmpEntries[0].(string)
	if !ok || !shortHashPattern.MatchString(strings.Split(tmpEntry, ":")[0]) || !strings.HasSuffix(tmpEntry, ":/tmp/lib/tool.jar") {
		t.Fatalf("steps[0].tmp[0] = %q, want canonical tmp entry", tmpEntry)
	}
}

func TestBuildSpecPayload_RelativePathsResolveFromSpecDir(t *testing.T) {
	specDir := t.TempDir()
	t.Chdir(t.TempDir())

	writeFile(t, filepath.Join(specDir, "step-fragment.yaml"), "image: docker.io/test/step-fragment:latest\n")

	result := buildAndParseSpec(t, specDir, `
envs:
  TOKEN: spec-dir-token
steps:
  - <<: !include step-fragment.yaml
`, ".yaml", specPayloadOpts{})

	envs := mustDig(t, result, "envs")
	assertField(t, envs, "TOKEN", "spec-dir-token")

	steps := mustSteps(t, result, 1)
	assertField(t, steps[0], "image", "docker.io/test/step-fragment:latest")
}

func TestBuildSpecPayload_IncludeCycleDetected(t *testing.T) {
	specDir := t.TempDir()
	writeFile(t, filepath.Join(specDir, "a.yaml"), "steps: !include ./b.yaml#/steps\n")
	writeFile(t, filepath.Join(specDir, "b.yaml"), "steps: !include ./a.yaml#/steps\n")

	specPath := filepath.Join(specDir, "spec.yaml")
	writeFile(t, specPath, `
steps: !include ./a.yaml#/steps
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
			name: "full CLI overrides on spec with envs",
			spec: `
steps:
  - image: docker.io/test/mig:v1
envs:
  KEY1: from_spec
  KEY2: value2
`,
			opts: specPayloadOpts{
				migEnvs:  []string{"KEY1=from_cli", "KEY3=new_value"},
				migImage: "docker.io/test/mig:v2",
				retain:   true,
			},
			wantStepCount:  1,
			wantStepImages: []string{"docker.io/test/mig:v2"},
			wantStepAbsent: []string{"retain_container"},
			wantTopEnv:     map[string]any{"KEY1": "from_cli", "KEY2": "value2", "KEY3": "new_value"},
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
	t.Setenv("PLOY_TEST_STEP_DEFAULT", "docker.io/test/default-step:latest")
	t.Setenv("PLOY_TEST_STEP_MAVEN", "docker.io/test/maven-step:latest")

	result := runBuildSpecPayload(t, `
steps:
  - image:
      default: $PLOY_TEST_STEP_DEFAULT
      java-maven: ${PLOY_TEST_STEP_MAVEN}
`, ".yaml", specPayloadOpts{})

	steps := mustSteps(t, result, 1)
	stepImage := mustDig(t, steps[0], "image")
	assertField(t, stepImage, "default", "docker.io/test/default-step:latest")
	assertField(t, stepImage, "java-maven", "docker.io/test/maven-step:latest")
}
