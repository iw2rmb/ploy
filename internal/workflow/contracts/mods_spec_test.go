package contracts

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestParseModsSpecJSON_SingleStep tests parsing single-step spec JSON.
func TestParseModsSpecJSON_SingleStep(t *testing.T) {
	input := `{
		"steps": [{
			"image": "docker.io/user/mod:latest",
			"command": "echo hello",
			"env": {"FOO": "bar", "BAZ": "qux"},
			"retain_container": true
		}],
		"build_gate": {"enabled": true},
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

	if len(spec.Steps) != 1 {
		t.Fatalf("len(steps) = %d, want 1", len(spec.Steps))
	}
	step := spec.Steps[0]

	// Verify command (shell form).
	if step.Command.Shell != "echo hello" {
		t.Errorf("command.Shell = %q, want %q", step.Command.Shell, "echo hello")
	}
	expected := []string{"/bin/sh", "-c", "echo hello"}
	got := step.Command.ToSlice()
	if len(got) != len(expected) {
		t.Errorf("Command.ToSlice() = %v, want %v", got, expected)
	}

	// Verify image (universal form).
	if step.Image.Universal != "docker.io/user/mod:latest" {
		t.Errorf("image = %q, want %q", step.Image.Universal, "docker.io/user/mod:latest")
	}

	// Verify env.
	if step.Env["FOO"] != "bar" {
		t.Errorf("env[FOO] = %q, want %q", step.Env["FOO"], "bar")
	}
	if step.Env["BAZ"] != "qux" {
		t.Errorf("env[BAZ] = %q, want %q", step.Env["BAZ"], "qux")
	}

	// Verify retain_container.
	if !step.RetainContainer {
		t.Errorf("retain_container = false, want true")
	}

	// Verify build_gate.
	if spec.BuildGate == nil {
		t.Fatal("build_gate is nil")
	}
	if !spec.BuildGate.Enabled {
		t.Errorf("build_gate.enabled = false, want true")
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
		"steps": [
			{"name": "step-1", "image": "docker.io/user/mod1:latest", "command": ["echo", "step1"], "env": {"STEP": "1"}},
			{"name": "step-2", "image": "docker.io/user/mod2:latest", "env": {"STEP": "2"}, "retain_container": true}
		],
		"build_gate": {
			"enabled": true,
			"healing": {
				"retries": 3,
				"image": "docker.io/user/codex:latest",
				"command": "fix-it",
				"env": {"PROMPT": "fix the build"}
			},
			"router": {
				"image": "docker.io/user/router:latest"
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

	// Verify steps array.
	if len(spec.Steps) != 2 {
		t.Fatalf("len(steps) = %d, want 2", len(spec.Steps))
	}

	// Verify first step.
	mod1 := spec.Steps[0]
	if mod1.Name != "step-1" {
		t.Errorf("steps[0].name = %q, want %q", mod1.Name, "step-1")
	}
	if mod1.Image.Universal != "docker.io/user/mod1:latest" {
		t.Errorf("steps[0].image = %q, want %q", mod1.Image.Universal, "docker.io/user/mod1:latest")
	}
	// Command is exec array form.
	if len(mod1.Command.Exec) != 2 || mod1.Command.Exec[0] != "echo" || mod1.Command.Exec[1] != "step1" {
		t.Errorf("steps[0].command.Exec = %v, want [echo, step1]", mod1.Command.Exec)
	}
	if mod1.Env["STEP"] != "1" {
		t.Errorf("steps[0].env[STEP] = %q, want %q", mod1.Env["STEP"], "1")
	}

	// Verify second step.
	mod2 := spec.Steps[1]
	if mod2.Name != "step-2" {
		t.Errorf("steps[1].name = %q, want %q", mod2.Name, "step-2")
	}
	if !mod2.RetainContainer {
		t.Errorf("steps[1].retain_container = false, want true")
	}

	// Verify healing.
	if spec.BuildGate == nil || spec.BuildGate.Healing == nil {
		t.Fatal("build_gate.healing is nil")
	}
	if spec.BuildGate.Healing.Retries != 3 {
		t.Errorf("build_gate.healing.retries = %d, want 3", spec.BuildGate.Healing.Retries)
	}
	if spec.BuildGate.Healing.Image.Universal != "docker.io/user/codex:latest" {
		t.Errorf("build_gate.healing.image = %q, want %q",
			spec.BuildGate.Healing.Image.Universal, "docker.io/user/codex:latest")
	}
	if spec.BuildGate.Healing.Command.Shell != "fix-it" {
		t.Errorf("build_gate.healing.command = %q, want %q",
			spec.BuildGate.Healing.Command.Shell, "fix-it")
	}
	if spec.BuildGate.Router == nil {
		t.Fatal("build_gate.router is nil")
	}
	if spec.BuildGate.Router.Image.Universal != "docker.io/user/router:latest" {
		t.Errorf("build_gate.router.image = %q, want %q",
			spec.BuildGate.Router.Image.Universal, "docker.io/user/router:latest")
	}
}

func TestParseModsSpecJSON_BuildGateStackConfig(t *testing.T) {
	input := `{
		"steps": [{
			"image": "docker.io/user/mod:latest"
		}],
		"build_gate": {
			"enabled": true,
			"pre": {
				"stack": {
					"enabled": true,
					"language": "java",
					"release": 11,
					"default": true
				}
			},
			"post": {
				"stack": {
					"enabled": true,
					"language": "java",
					"tool": "maven",
					"release": "17",
					"default": false
				}
			}
		}
	}`

	spec, err := ParseModsSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	if spec.BuildGate == nil {
		t.Fatal("build_gate is nil")
	}
	if spec.BuildGate.Pre == nil || spec.BuildGate.Pre.Stack == nil {
		t.Fatal("build_gate.pre.stack is nil")
	}
	if !spec.BuildGate.Pre.Stack.Enabled {
		t.Errorf("build_gate.pre.stack.enabled = false, want true")
	}
	if spec.BuildGate.Pre.Stack.Language != "java" {
		t.Errorf("build_gate.pre.stack.language = %q, want %q", spec.BuildGate.Pre.Stack.Language, "java")
	}
	if spec.BuildGate.Pre.Stack.Release != "11" {
		t.Errorf("build_gate.pre.stack.release = %q, want %q", spec.BuildGate.Pre.Stack.Release, "11")
	}
	if !spec.BuildGate.Pre.Stack.Default {
		t.Errorf("build_gate.pre.stack.default = false, want true")
	}

	if spec.BuildGate.Post == nil || spec.BuildGate.Post.Stack == nil {
		t.Fatal("build_gate.post.stack is nil")
	}
	if spec.BuildGate.Post.Stack.Tool != "maven" {
		t.Errorf("build_gate.post.stack.tool = %q, want %q", spec.BuildGate.Post.Stack.Tool, "maven")
	}
	if spec.BuildGate.Post.Stack.Release != "17" {
		t.Errorf("build_gate.post.stack.release = %q, want %q", spec.BuildGate.Post.Stack.Release, "17")
	}
	if spec.BuildGate.Post.Stack.Default {
		t.Errorf("build_gate.post.stack.default = true, want false")
	}
}

func TestParseModsSpecJSON_BuildGateStackConfig_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "enabled without language",
			input: `{
				"steps": [{"image": "docker.io/user/mod:latest"}],
				"build_gate": {"pre": {"stack": {"enabled": true, "release": "11"}}}
			}`,
			wantErr: "build_gate.pre.stack.language: required",
		},
		{
			name: "enabled without release",
			input: `{
				"steps": [{"image": "docker.io/user/mod:latest"}],
				"build_gate": {"post": {"stack": {"enabled": true, "language": "java"}}}
			}`,
			wantErr: "build_gate.post.stack.release: required",
		},
		{
			name: "disabled with fields is ambiguous",
			input: `{
				"steps": [{"image": "docker.io/user/mod:latest"}],
				"build_gate": {"pre": {"stack": {"enabled": false, "language": "java", "release": "11"}}}
			}`,
			wantErr: "build_gate.pre.stack: enabled=false with stack fields is ambiguous",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseModsSpecJSON([]byte(tt.input))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestParseModsSpecJSON_StackSpecificImage tests stack-specific image parsing.
func TestParseModsSpecJSON_StackSpecificImage(t *testing.T) {
	input := `{
		"steps": [{
			"image": {
				"default": "docker.io/user/mod:default",
				"java-maven": "docker.io/user/mod:maven",
				"java-gradle": "docker.io/user/mod:gradle"
			}
		}]
	}`

	spec, err := ParseModsSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	if spec.Steps[0].Image.IsUniversal() {
		t.Errorf("expected stack-specific image, got universal")
	}
	if !spec.Steps[0].Image.IsStackSpecific() {
		t.Errorf("expected IsStackSpecific() = true")
	}

	// Verify resolution.
	img, err := spec.Steps[0].Image.ResolveImage(ModStackJavaMaven)
	if err != nil {
		t.Fatalf("ResolveImage(java-maven) failed: %v", err)
	}
	if img != "docker.io/user/mod:maven" {
		t.Errorf("ResolveImage(java-maven) = %q, want %q", img, "docker.io/user/mod:maven")
	}

	// Verify default fallback.
	img, err = spec.Steps[0].Image.ResolveImage(ModStackUnknown)
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
		"steps": [{
			"image": "docker.io/user/mod:latest",
			"command": "echo hello",
			"env": {"FOO": "bar"}
		}],
		"build_gate": {"enabled": true}
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
	if spec.Steps[0].Image.Universal != "docker.io/user/mod:latest" {
		t.Errorf("image = %q, want %q", spec.Steps[0].Image.Universal, "docker.io/user/mod:latest")
	}
	if spec.Steps[0].Command.Shell != "echo hello" {
		t.Errorf("command = %q, want %q", spec.Steps[0].Command.Shell, "echo hello")
	}
}

// TestParseModsSpecJSON_Empty tests empty input handling.
func TestParseModsSpecJSON_Empty(t *testing.T) {
	_, err := ParseModsSpecJSON(nil)
	if err == nil {
		t.Fatal("expected error for empty input")
	}
	if want := "steps: required"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestParseModsSpecJSON_ModIndexForbidden(t *testing.T) {
	input := `{"mod_index":0,"steps":[{"image":"docker.io/user/mod:latest"}]}`
	_, err := ParseModsSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected error for mod_index")
	}
	if !strings.Contains(err.Error(), "mod_index: forbidden") {
		t.Fatalf("expected mod_index forbidden error, got %q", err.Error())
	}
}

// TestParseModsSpecJSON_ValidationError tests validation errors.
func TestParseModsSpecJSON_ValidationError(t *testing.T) {
	// Step without image.
	input := `{"steps": [{"name": "test"}]}`
	_, err := ParseModsSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected validation error for mod without image")
	}
	if want := "steps[0].image: required"; err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// TestParseModsSpecJSON_HealingValidation tests healing spec validation.
func TestParseModsSpecJSON_HealingValidation(t *testing.T) {
	// Healing with image but no router.
	input := `{
		"steps": [{"image": "test:latest"}],
		"build_gate": {"healing": {"retries": 1, "image": "codex:latest", "command": "fix"}}
	}`
	_, err := ParseModsSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected validation error for healing without router")
	}
}

func TestParseModsSpecJSON_HealingRequiresImage(t *testing.T) {
	// Healing configured without an image.
	input := `{
		"steps": [{"image": "test:latest"}],
		"build_gate": {
			"healing": {"retries": 1},
			"router": {"image": "router:latest"}
		}
	}`
	_, err := ParseModsSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected validation error for healing without image")
	}
	if want := "build_gate.healing.image: required when healing is configured"; err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
}

// TestModsSpec_ToMap tests round-trip conversion via ToMap.
func TestModsSpec_ToMap(t *testing.T) {
	mrOnSuccess := true
	original := &ModsSpec{
		Steps: []ModStep{{
			Image:           JobImage{Universal: "docker.io/user/mod:latest"},
			Command:         CommandSpec{Shell: "echo hello"},
			Env:             map[string]string{"FOO": "bar"},
			RetainContainer: true,
		}},
		BuildGate:   &BuildGateConfig{Enabled: true},
		GitLabPAT:   "secret",
		MROnSuccess: &mrOnSuccess,
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
	if parsed.Steps[0].Image.Universal != original.Steps[0].Image.Universal {
		t.Errorf("image = %q, want %q", parsed.Steps[0].Image.Universal, original.Steps[0].Image.Universal)
	}
	if parsed.Steps[0].Command.Shell != original.Steps[0].Command.Shell {
		t.Errorf("command.Shell = %q, want %q", parsed.Steps[0].Command.Shell, original.Steps[0].Command.Shell)
	}
	if !parsed.Steps[0].RetainContainer {
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
		Steps: []ModStep{
			{Name: "step-1", Image: JobImage{Universal: "mod1:latest"}},
			{Name: "step-2", Image: JobImage{ByStack: map[ModStack]string{
				ModStackDefault:    "mod2:default",
				ModStackJavaMaven:  "mod2:maven",
				ModStackJavaGradle: "mod2:gradle",
			}}},
		},
		BuildGate: &BuildGateConfig{
			Healing: &HealingSpec{
				Retries: 2,
				Image:   JobImage{Universal: "codex:latest"},
			},
			Router: &RouterSpec{
				Image: JobImage{Universal: "router:latest"},
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
	if len(parsed.Steps) != 2 {
		t.Fatalf("len(steps) = %d, want 2", len(parsed.Steps))
	}
	if parsed.Steps[0].Name != "step-1" {
		t.Errorf("steps[0].name = %q, want %q", parsed.Steps[0].Name, "step-1")
	}
	if !parsed.Steps[1].Image.IsStackSpecific() {
		t.Errorf("steps[1].image should be stack-specific")
	}
	if parsed.BuildGate == nil || parsed.BuildGate.Healing == nil || parsed.BuildGate.Healing.Retries != 2 {
		t.Errorf("build_gate.healing.retries should be 2")
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
		"steps": [{"image": "test:latest"}],
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

func TestParseModsSpecJSON_RequiresStepsEvenWithExtraFields(t *testing.T) {
	input := `{
		"mod": {
			"image": "docker.io/user/mod:latest",
			"command": "echo hello"
		}
	}`

	_, err := ParseModsSpecJSON([]byte(input))
	if err == nil {
		t.Fatal("expected error for missing steps")
	}
	wantErr := "steps: required"
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
