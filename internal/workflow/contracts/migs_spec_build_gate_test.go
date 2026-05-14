package contracts

import (
	"strings"
	"testing"
)

func TestParseMigSpecJSON_BuildGateStackConfig(t *testing.T) {
	input := `{
		"steps": [{
			"image": "ghcr.io/iw2rmb/ploy/mig:latest"
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

func TestParseMigSpecJSON_BuildGateStackConfig_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "enabled without language",
			input: `{
				"steps": [{"image": "ghcr.io/iw2rmb/ploy/mig:latest"}],
				"build_gate": {"pre": {"stack": {"enabled": true, "release": "11"}}}
			}`,
			wantErr: "build_gate.pre.stack.language: required",
		},
		{
			name: "enabled without release",
			input: `{
				"steps": [{"image": "ghcr.io/iw2rmb/ploy/mig:latest"}],
				"build_gate": {"post": {"stack": {"enabled": true, "language": "java"}}}
			}`,
			wantErr: "build_gate.post.stack.release: required",
		},
		{
			name: "disabled with fields is ambiguous",
			input: `{
				"steps": [{"image": "ghcr.io/iw2rmb/ploy/mig:latest"}],
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

// TestParseMigSpecJSON_StackSpecificImage tests stack-specific image parsing.
