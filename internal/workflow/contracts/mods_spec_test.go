package contracts

import (
	"encoding/json"
	"testing"
)

// TestParseModsSpecJSON_SingleStep tests parsing single-step spec JSON.
func TestParseModsSpecJSON_SingleStep(t *testing.T) {
	input := `{
		"image": "docker.io/user/mod:latest",
		"command": "echo hello",
		"env": {"FOO": "bar", "BAZ": "qux"},
		"retain_container": true,
		"build_gate": {"enabled": true, "profile": "java-maven"},
		"gitlab_pat": "secret",
		"gitlab_domain": "gitlab.com",
		"mr_on_success": true,
		"mr_on_fail": false
	}`

	spec, err := ParseModsSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	// Verify single-step detection.
	if !spec.IsSingleStep() {
		t.Errorf("expected IsSingleStep() = true")
	}
	if spec.IsMultiStep() {
		t.Errorf("expected IsMultiStep() = false")
	}

	// Verify image (universal form).
	if spec.Image.Universal != "docker.io/user/mod:latest" {
		t.Errorf("image = %q, want %q", spec.Image.Universal, "docker.io/user/mod:latest")
	}

	// Verify command (shell form).
	if spec.Command.Shell != "echo hello" {
		t.Errorf("command.Shell = %q, want %q", spec.Command.Shell, "echo hello")
	}
	expected := []string{"/bin/sh", "-c", "echo hello"}
	got := spec.Command.ToSlice()
	if len(got) != len(expected) {
		t.Errorf("Command.ToSlice() = %v, want %v", got, expected)
	}

	// Verify env.
	if spec.Env["FOO"] != "bar" {
		t.Errorf("env[FOO] = %q, want %q", spec.Env["FOO"], "bar")
	}
	if spec.Env["BAZ"] != "qux" {
		t.Errorf("env[BAZ] = %q, want %q", spec.Env["BAZ"], "qux")
	}

	// Verify retain_container.
	if !spec.RetainContainer {
		t.Errorf("retain_container = false, want true")
	}

	// Verify build_gate.
	if spec.BuildGate == nil {
		t.Fatal("build_gate is nil")
	}
	if !spec.BuildGate.Enabled {
		t.Errorf("build_gate.enabled = false, want true")
	}
	if spec.BuildGate.Profile != "java-maven" {
		t.Errorf("build_gate.profile = %q, want %q", spec.BuildGate.Profile, "java-maven")
	}

	// Verify GitLab integration.
	if spec.GitLabPAT != "secret" {
		t.Errorf("gitlab_pat = %q, want %q", spec.GitLabPAT, "secret")
	}
	if spec.GitLabDomain != "gitlab.com" {
		t.Errorf("gitlab_domain = %q, want %q", spec.GitLabDomain, "gitlab.com")
	}
	if spec.MROnSuccess == nil || !*spec.MROnSuccess {
		t.Errorf("mr_on_success = %v, want true", spec.MROnSuccess)
	}
	if spec.MROnFail == nil || *spec.MROnFail {
		t.Errorf("mr_on_fail = %v, want false", spec.MROnFail)
	}
}

// TestParseModsSpecJSON_MultiStep tests parsing multi-step spec JSON.
func TestParseModsSpecJSON_MultiStep(t *testing.T) {
	input := `{
		"mods": [
			{"name": "step-1", "image": "docker.io/user/mod1:latest", "command": ["echo", "step1"], "env": {"STEP": "1"}},
			{"name": "step-2", "image": "docker.io/user/mod2:latest", "env": {"STEP": "2"}, "retain_container": true}
		],
		"build_gate": {"enabled": true, "profile": "auto"},
		"build_gate_healing": {
			"retries": 3,
			"mod": {
				"image": "docker.io/user/codex:latest",
				"command": "fix-it",
				"env": {"PROMPT": "fix the build"}
			}
		}
	}`

	spec, err := ParseModsSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	// Verify multi-step detection.
	if spec.IsSingleStep() {
		t.Errorf("expected IsSingleStep() = false")
	}
	if !spec.IsMultiStep() {
		t.Errorf("expected IsMultiStep() = true")
	}

	// Verify mods array.
	if len(spec.Mods) != 2 {
		t.Fatalf("len(mods) = %d, want 2", len(spec.Mods))
	}

	// Verify first mod.
	mod1 := spec.Mods[0]
	if mod1.Name != "step-1" {
		t.Errorf("mods[0].name = %q, want %q", mod1.Name, "step-1")
	}
	if mod1.Image.Universal != "docker.io/user/mod1:latest" {
		t.Errorf("mods[0].image = %q, want %q", mod1.Image.Universal, "docker.io/user/mod1:latest")
	}
	// Command is exec array form.
	if len(mod1.Command.Exec) != 2 || mod1.Command.Exec[0] != "echo" || mod1.Command.Exec[1] != "step1" {
		t.Errorf("mods[0].command.Exec = %v, want [echo, step1]", mod1.Command.Exec)
	}
	if mod1.Env["STEP"] != "1" {
		t.Errorf("mods[0].env[STEP] = %q, want %q", mod1.Env["STEP"], "1")
	}

	// Verify second mod.
	mod2 := spec.Mods[1]
	if mod2.Name != "step-2" {
		t.Errorf("mods[1].name = %q, want %q", mod2.Name, "step-2")
	}
	if !mod2.RetainContainer {
		t.Errorf("mods[1].retain_container = false, want true")
	}

	// Verify healing.
	if spec.BuildGateHealing == nil {
		t.Fatal("build_gate_healing is nil")
	}
	if spec.BuildGateHealing.Retries != 3 {
		t.Errorf("build_gate_healing.retries = %d, want 3", spec.BuildGateHealing.Retries)
	}
	if spec.BuildGateHealing.Mod == nil {
		t.Fatal("build_gate_healing.mod is nil")
	}
	if spec.BuildGateHealing.Mod.Image.Universal != "docker.io/user/codex:latest" {
		t.Errorf("build_gate_healing.mod.image = %q, want %q",
			spec.BuildGateHealing.Mod.Image.Universal, "docker.io/user/codex:latest")
	}
	if spec.BuildGateHealing.Mod.Command.Shell != "fix-it" {
		t.Errorf("build_gate_healing.mod.command = %q, want %q",
			spec.BuildGateHealing.Mod.Command.Shell, "fix-it")
	}
}

// TestParseModsSpecJSON_StackSpecificImage tests stack-specific image parsing.
func TestParseModsSpecJSON_StackSpecificImage(t *testing.T) {
	input := `{
		"image": {
			"default": "docker.io/user/mod:default",
			"java-maven": "docker.io/user/mod:maven",
			"java-gradle": "docker.io/user/mod:gradle"
		}
	}`

	spec, err := ParseModsSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	if spec.Image.IsUniversal() {
		t.Errorf("expected stack-specific image, got universal")
	}
	if !spec.Image.IsStackSpecific() {
		t.Errorf("expected IsStackSpecific() = true")
	}

	// Verify resolution.
	img, err := spec.Image.ResolveImage(ModStackJavaMaven)
	if err != nil {
		t.Fatalf("ResolveImage(java-maven) failed: %v", err)
	}
	if img != "docker.io/user/mod:maven" {
		t.Errorf("ResolveImage(java-maven) = %q, want %q", img, "docker.io/user/mod:maven")
	}

	// Verify default fallback.
	img, err = spec.Image.ResolveImage(ModStackUnknown)
	if err != nil {
		t.Fatalf("ResolveImage(unknown) failed: %v", err)
	}
	if img != "docker.io/user/mod:default" {
		t.Errorf("ResolveImage(unknown) = %q, want %q", img, "docker.io/user/mod:default")
	}
}

// TestParseModsSpecJSON_APIVersionAndKind tests parsing of optional metadata fields.
// These fields are informational (typically from YAML manifests converted to JSON).
func TestParseModsSpecJSON_APIVersionAndKind(t *testing.T) {
	input := `{
		"apiVersion": "ploy.mod/v1alpha1",
		"kind": "ModRunSpec",
		"image": "docker.io/user/mod:latest",
		"command": "echo hello",
		"env": {"FOO": "bar"},
		"build_gate": {"enabled": true, "profile": "auto"}
	}`

	spec, err := ParseModsSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	if spec.APIVersion != "ploy.mod/v1alpha1" {
		t.Errorf("apiVersion = %q, want %q", spec.APIVersion, "ploy.mod/v1alpha1")
	}
	if spec.Kind != "ModRunSpec" {
		t.Errorf("kind = %q, want %q", spec.Kind, "ModRunSpec")
	}
	if spec.Image.Universal != "docker.io/user/mod:latest" {
		t.Errorf("image = %q, want %q", spec.Image.Universal, "docker.io/user/mod:latest")
	}
	if spec.Command.Shell != "echo hello" {
		t.Errorf("command = %q, want %q", spec.Command.Shell, "echo hello")
	}
}

// TestParseModsSpecJSON_Empty tests empty input handling.
func TestParseModsSpecJSON_Empty(t *testing.T) {
	spec, err := ParseModsSpecJSON(nil)
	if err != nil {
		t.Fatalf("ParseModsSpecJSON(nil) failed: %v", err)
	}
	if spec == nil {
		t.Fatal("expected non-nil spec for empty input")
	}
	if err := spec.Validate(); err != nil {
		t.Errorf("empty spec should validate: %v", err)
	}
}

// TestParseModsSpecJSON_ValidationError tests validation errors.
func TestParseModsSpecJSON_ValidationError(t *testing.T) {
	// Multi-step mod without image.
	input := `{"mods": [{"name": "test"}]}`
	_, err := ParseModsSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected validation error for mod without image")
	}
	if want := "mods[0]: image is required"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// TestParseModsSpecJSON_HealingValidation tests healing spec validation.
func TestParseModsSpecJSON_HealingValidation(t *testing.T) {
	// Healing mod without image.
	input := `{
		"image": "test:latest",
		"build_gate_healing": {"retries": 1, "mod": {"command": "fix"}}
	}`
	_, err := ParseModsSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected validation error for healing mod without image")
	}
}

// TestModsSpec_ToMap tests round-trip conversion via ToMap.
func TestModsSpec_ToMap(t *testing.T) {
	mrOnSuccess := true
	original := &ModsSpec{
		Image:           ModImage{Universal: "docker.io/user/mod:latest"},
		Command:         CommandSpec{Shell: "echo hello"},
		Env:             map[string]string{"FOO": "bar"},
		RetainContainer: true,
		BuildGate:       &BuildGateConfig{Enabled: true, Profile: "auto"},
		GitLabPAT:       "secret",
		MROnSuccess:     &mrOnSuccess,
	}

	m := original.ToMap()

	// Marshal to JSON and parse back.
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	parsed, err := ParseModsSpecJSON(data)
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	// Verify round-trip.
	if parsed.Image.Universal != original.Image.Universal {
		t.Errorf("image = %q, want %q", parsed.Image.Universal, original.Image.Universal)
	}
	if parsed.Command.Shell != original.Command.Shell {
		t.Errorf("command.Shell = %q, want %q", parsed.Command.Shell, original.Command.Shell)
	}
	if !parsed.RetainContainer {
		t.Errorf("retain_container = false, want true")
	}
	if parsed.BuildGate == nil || !parsed.BuildGate.Enabled {
		t.Errorf("build_gate.enabled should be true")
	}
	if parsed.GitLabPAT != "secret" {
		t.Errorf("gitlab_pat = %q, want %q", parsed.GitLabPAT, "secret")
	}
}

// TestModsSpec_ToMap_MultiStep tests ToMap for multi-step specs.
func TestModsSpec_ToMap_MultiStep(t *testing.T) {
	original := &ModsSpec{
		Mods: []ModStep{
			{Name: "step-1", Image: ModImage{Universal: "mod1:latest"}},
			{Name: "step-2", Image: ModImage{ByStack: map[ModStack]string{
				ModStackDefault:    "mod2:default",
				ModStackJavaMaven:  "mod2:maven",
				ModStackJavaGradle: "mod2:gradle",
			}}},
		},
		BuildGateHealing: &HealingSpec{
			Retries: 2,
			Mod: &HealingModSpec{
				Image: ModImage{Universal: "codex:latest"},
			},
		},
	}

	m := original.ToMap()

	// Marshal to JSON and parse back.
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	parsed, err := ParseModsSpecJSON(data)
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	// Verify round-trip.
	if len(parsed.Mods) != 2 {
		t.Fatalf("len(mods) = %d, want 2", len(parsed.Mods))
	}
	if parsed.Mods[0].Name != "step-1" {
		t.Errorf("mods[0].name = %q, want %q", parsed.Mods[0].Name, "step-1")
	}
	if !parsed.Mods[1].Image.IsStackSpecific() {
		t.Errorf("mods[1].image should be stack-specific")
	}
	if parsed.BuildGateHealing == nil || parsed.BuildGateHealing.Retries != 2 {
		t.Errorf("build_gate_healing.retries should be 2")
	}
}

// TestCommandSpec_ToSlice tests command conversion to slice.
func TestCommandSpec_ToSlice(t *testing.T) {
	tests := []struct {
		name string
		cmd  CommandSpec
		want []string
	}{
		{
			name: "shell string",
			cmd:  CommandSpec{Shell: "echo hello"},
			want: []string{"/bin/sh", "-c", "echo hello"},
		},
		{
			name: "exec array",
			cmd:  CommandSpec{Exec: []string{"echo", "hello"}},
			want: []string{"echo", "hello"},
		},
		{
			name: "empty",
			cmd:  CommandSpec{},
			want: nil,
		},
		{
			name: "exec takes precedence",
			cmd:  CommandSpec{Shell: "ignored", Exec: []string{"echo", "used"}},
			want: []string{"echo", "used"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cmd.ToSlice()
			if len(got) != len(tt.want) {
				t.Errorf("ToSlice() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ToSlice()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestCommandSpec_JSONMarshal tests JSON marshaling of CommandSpec.
func TestCommandSpec_JSONMarshal(t *testing.T) {
	tests := []struct {
		name string
		cmd  CommandSpec
		want string
	}{
		{
			name: "shell string",
			cmd:  CommandSpec{Shell: "echo hello"},
			want: `"echo hello"`,
		},
		{
			name: "exec array",
			cmd:  CommandSpec{Exec: []string{"echo", "hello"}},
			want: `["echo","hello"]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.cmd)
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}
			if string(data) != tt.want {
				t.Errorf("json.Marshal() = %s, want %s", data, tt.want)
			}
		})
	}
}

// TestCommandSpec_JSONUnmarshal tests JSON unmarshaling of CommandSpec.
func TestCommandSpec_JSONUnmarshal(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantShell string
		wantExec  []string
	}{
		{
			name:      "shell string",
			input:     `"echo hello"`,
			wantShell: "echo hello",
		},
		{
			name:     "exec array",
			input:    `["echo", "hello"]`,
			wantExec: []string{"echo", "hello"},
		},
		{
			name:  "null",
			input: `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cmd CommandSpec
			if err := json.Unmarshal([]byte(tt.input), &cmd); err != nil {
				t.Fatalf("json.Unmarshal failed: %v", err)
			}
			if cmd.Shell != tt.wantShell {
				t.Errorf("Shell = %q, want %q", cmd.Shell, tt.wantShell)
			}
			if len(cmd.Exec) != len(tt.wantExec) {
				t.Errorf("Exec = %v, want %v", cmd.Exec, tt.wantExec)
			}
		})
	}
}

// TestModsSpec_ArtifactFields tests artifact configuration parsing.
func TestModsSpec_ArtifactFields(t *testing.T) {
	input := `{
		"image": "test:latest",
		"artifact_name": "my-bundle",
		"artifact_paths": ["output/", "logs/app.log"]
	}`

	spec, err := ParseModsSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	if spec.ArtifactName != "my-bundle" {
		t.Errorf("artifact_name = %q, want %q", spec.ArtifactName, "my-bundle")
	}
	if len(spec.ArtifactPaths) != 2 {
		t.Fatalf("len(artifact_paths) = %d, want 2", len(spec.ArtifactPaths))
	}
	if spec.ArtifactPaths[0] != "output/" {
		t.Errorf("artifact_paths[0] = %q, want %q", spec.ArtifactPaths[0], "output/")
	}
}

// TestParseModsSpecJSON_RejectsLegacyModShape tests that the parser rejects
// the legacy top-level "mod" section shape. The canonical spec supports only:
// 1. Single-step: top-level image/command/env/retain_container
// 2. Multi-step: mods[] array
// The legacy "mod: {image: ...}" shape is explicitly rejected to prevent
// silent no-op parsing and ensure documentation-parser alignment.
func TestParseModsSpecJSON_RejectsLegacyModShape(t *testing.T) {
	// Legacy spec shape with top-level "mod" section (not supported).
	input := `{
		"mod": {
			"image": "docker.io/user/mod:latest",
			"command": "echo hello"
		}
	}`

	_, err := ParseModsSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected error for legacy 'mod' section shape")
	}
	wantErr := "mod: legacy spec shape is not supported; use top-level fields or mods[]"
	if err.Error() != wantErr {
		t.Errorf("error = %q, want %q", err.Error(), wantErr)
	}
}

// TestParseModsSpecJSON_InvalidJSON tests error handling for invalid JSON.
func TestParseModsSpecJSON_InvalidJSON(t *testing.T) {
	_, err := ParseModsSpecJSON([]byte(`{invalid json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
