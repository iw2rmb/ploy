package contracts

import (
	"strings"
	"testing"
)

func TestParseModsSpecJSON_BuildGateStackConfig(t *testing.T) {
	input := `{
		"steps": [{
			"image": "docker.io/user/mig:latest"
		}],
		"build_gate": {
			"enabled": true,
			"pre": {
				"target": "unit",
				"always": true,
				"stack": {
					"enabled": true,
					"language": "java",
					"release": 11,
					"default": true
				}
			},
			"post": {
				"target": "all_tests",
				"always": false,
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

	spec, err := ParseMigSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseMigSpecJSON failed: %v", err)
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
	if spec.BuildGate.Pre.Target != GateProfileTargetUnit {
		t.Errorf("build_gate.pre.target = %q, want %q", spec.BuildGate.Pre.Target, GateProfileTargetUnit)
	}
	if !spec.BuildGate.Pre.Always {
		t.Errorf("build_gate.pre.always = false, want true")
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
	if spec.BuildGate.Post.Target != GateProfileTargetAllTests {
		t.Errorf("build_gate.post.target = %q, want %q", spec.BuildGate.Post.Target, GateProfileTargetAllTests)
	}
	if spec.BuildGate.Post.Always {
		t.Errorf("build_gate.post.always = true, want false")
	}
}

func TestParseModsSpecJSON_BuildGateStackConfig_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "invalid gate target",
			input: `{
				"steps": [{"image": "docker.io/user/mig:latest"}],
				"build_gate": {"pre": {"target": "unsupported"}}
			}`,
			wantErr: "build_gate.pre.target: invalid value",
		},
		{
			name: "enabled without language",
			input: `{
				"steps": [{"image": "docker.io/user/mig:latest"}],
				"build_gate": {"pre": {"stack": {"enabled": true, "release": "11"}}}
			}`,
			wantErr: "build_gate.pre.stack.language: required",
		},
		{
			name: "enabled without release",
			input: `{
				"steps": [{"image": "docker.io/user/mig:latest"}],
				"build_gate": {"post": {"stack": {"enabled": true, "language": "java"}}}
			}`,
			wantErr: "build_gate.post.stack.release: required",
		},
		{
			name: "disabled with fields is ambiguous",
			input: `{
				"steps": [{"image": "docker.io/user/mig:latest"}],
				"build_gate": {"pre": {"stack": {"enabled": false, "language": "java", "release": "11"}}}
			}`,
			wantErr: "build_gate.pre.stack: enabled=false with stack fields is ambiguous",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseMigSpecJSON([]byte(tt.input))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestParseModsSpecJSON_BuildGateProfileOverride(t *testing.T) {
	input := `{
		"steps": [{
			"image": "docker.io/user/mig:latest"
		}],
		"build_gate": {
			"pre": {
				"gate_profile": {
					"command": "go test ./...",
					"env": {
						"GOFLAGS": "-mod=readonly"
					}
				}
			},
			"post": {
				"gate_profile": {
					"command": ["go", "test", "./...", "-run", "TestUnit"],
					"env": {
						"CGO_ENABLED": "0"
					}
				}
			}
		}
	}`

	spec, err := ParseMigSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseMigSpecJSON failed: %v", err)
	}
	if spec.BuildGate == nil || spec.BuildGate.Pre == nil || spec.BuildGate.Pre.GateProfile == nil {
		t.Fatal("build_gate.pre.gate_profile is nil")
	}
	if spec.BuildGate.Pre.GateProfile.Command.Shell != "go test ./..." {
		t.Fatalf("build_gate.pre.gate_profile.command.shell = %q, want %q", spec.BuildGate.Pre.GateProfile.Command.Shell, "go test ./...")
	}
	if got := spec.BuildGate.Pre.GateProfile.Env["GOFLAGS"]; got != "-mod=readonly" {
		t.Fatalf("build_gate.pre.gate_profile.env[GOFLAGS] = %q, want %q", got, "-mod=readonly")
	}

	if spec.BuildGate.Post == nil || spec.BuildGate.Post.GateProfile == nil {
		t.Fatal("build_gate.post.gate_profile is nil")
	}
	wantPost := []string{"go", "test", "./...", "-run", "TestUnit"}
	if len(spec.BuildGate.Post.GateProfile.Command.Exec) != len(wantPost) {
		t.Fatalf("build_gate.post.gate_profile.command.exec length = %d, want %d", len(spec.BuildGate.Post.GateProfile.Command.Exec), len(wantPost))
	}
	for i, v := range wantPost {
		if got := spec.BuildGate.Post.GateProfile.Command.Exec[i]; got != v {
			t.Fatalf("build_gate.post.gate_profile.command.exec[%d] = %q, want %q", i, got, v)
		}
	}
	if got := spec.BuildGate.Post.GateProfile.Env["CGO_ENABLED"]; got != "0" {
		t.Fatalf("build_gate.post.gate_profile.env[CGO_ENABLED] = %q, want %q", got, "0")
	}
}

func TestParseModsSpecJSON_BuildGateProfileOverride_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "pre gate_profile missing command",
			input: `{
				"steps": [{"image": "docker.io/user/mig:latest"}],
				"build_gate": {"pre": {"gate_profile": {"env": {"A": "B"}}}}
			}`,
			wantErr: "build_gate.pre.gate_profile.command: required",
		},
		{
			name: "post gate_profile command wrong type",
			input: `{
				"steps": [{"image": "docker.io/user/mig:latest"}],
				"build_gate": {"post": {"gate_profile": {"command": {"bad": true}}}}
			}`,
			wantErr: "command: expected string or array",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseMigSpecJSON([]byte(tt.input))
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
