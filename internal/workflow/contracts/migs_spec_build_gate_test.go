package contracts

import (
	"strings"
	"testing"
)

func TestParseMigSpecJSON_BuildGateStackConfig(t *testing.T) {
	tests := []struct {
		name        string
		mode        BuildGateStackMode
		stackJSON   string
		wantTool    string
		wantRelease string
	}{
		{name: "forced", mode: BuildGateStackModeForced, stackJSON: `"language": "java", "tool": "maven", "release": 11`, wantTool: "maven", wantRelease: "11"},
		{name: "strict full tuple", mode: BuildGateStackModeStrict, stackJSON: `"language": "java", "tool": "maven", "release": "17"`, wantTool: "maven", wantRelease: "17"},
		{name: "strict partial tuple", mode: BuildGateStackModeStrict, stackJSON: `"language": "java", "release": "17"`, wantRelease: "17"},
		{name: "fallback", mode: BuildGateStackModeFallback, stackJSON: `"language": "java", "tool": "maven", "release": "21"`, wantTool: "maven", wantRelease: "21"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `{
				"steps": [{
					"image": "ghcr.io/iw2rmb/ploy/mig:latest"
				}],
				"build_gate": {
					"pre": {
						"stack": {
							"mode": "` + string(tt.mode) + `",
							` + tt.stackJSON + `
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
			stack := spec.BuildGate.Pre.Stack
			if stack.Mode != tt.mode {
				t.Errorf("build_gate.pre.stack.mode = %q, want %q", stack.Mode, tt.mode)
			}
			if stack.Language != "java" {
				t.Errorf("build_gate.pre.stack.language = %q, want %q", stack.Language, "java")
			}
			if stack.Tool != tt.wantTool {
				t.Errorf("build_gate.pre.stack.tool = %q, want %q", stack.Tool, tt.wantTool)
			}
			if stack.Release != tt.wantRelease {
				t.Errorf("build_gate.pre.stack.release = %q, want %q", stack.Release, tt.wantRelease)
			}
		})
	}
}

func TestParseMigSpecJSON_BuildGateStackConfig_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "fallback without language",
			input: `{
				"steps": [{"image": "ghcr.io/iw2rmb/ploy/mig:latest"}],
				"build_gate": {"pre": {"stack": {"mode": "fallback", "tool": "maven", "release": "11"}}}
			}`,
			wantErr: "build_gate.pre.stack",
		},
		{
			name: "forced without tool",
			input: `{
				"steps": [{"image": "ghcr.io/iw2rmb/ploy/mig:latest"}],
				"build_gate": {"post": {"stack": {"mode": "forced", "language": "java", "release": "17"}}}
			}`,
			wantErr: "build_gate.post.stack",
		},
		{
			name: "fallback without release",
			input: `{
				"steps": [{"image": "ghcr.io/iw2rmb/ploy/mig:latest"}],
				"build_gate": {"post": {"stack": {"mode": "fallback", "language": "java", "tool": "maven"}}}
			}`,
			wantErr: "build_gate.post.stack",
		},
		{
			name: "strict without expectation",
			input: `{
				"steps": [{"image": "ghcr.io/iw2rmb/ploy/mig:latest"}],
				"build_gate": {"post": {"stack": {"mode": "strict"}}}
			}`,
			wantErr: "build_gate.post.stack",
		},
		{
			name: "stack fields without mode",
			input: `{
				"steps": [{"image": "ghcr.io/iw2rmb/ploy/mig:latest"}],
				"build_gate": {"pre": {"stack": {"language": "java", "tool": "maven", "release": "11"}}}
			}`,
			wantErr: "build_gate.pre.stack",
		},
		{
			name: "unknown mode",
			input: `{
				"steps": [{"image": "ghcr.io/iw2rmb/ploy/mig:latest"}],
				"build_gate": {"pre": {"stack": {"mode": "prefer", "language": "java", "tool": "maven", "release": "11"}}}
			}`,
			wantErr: "build_gate.pre.stack.mode",
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
