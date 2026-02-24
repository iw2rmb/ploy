package contracts

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestBuildGateImageRule_Specificity tests specificity calculation.
func TestBuildGateImageRule_Specificity(t *testing.T) {
	tests := []struct {
		name string
		rule BuildGateImageRule
		want int
	}{
		{
			name: "tool-specific (language+tool+release)",
			rule: BuildGateImageRule{
				Stack: StackExpectation{Language: "java", Tool: "maven", Release: "17"},
				Image: "maven:3-eclipse-temurin-17",
			},
			want: 3,
		},
		{
			name: "tool-agnostic (language+release)",
			rule: BuildGateImageRule{
				Stack: StackExpectation{Language: "java", Release: "17"},
				Image: "eclipse-temurin:17-jdk",
			},
			want: 2,
		},
		{
			name: "empty tool is tool-agnostic",
			rule: BuildGateImageRule{
				Stack: StackExpectation{Language: "java", Tool: "", Release: "17"},
				Image: "eclipse-temurin:17-jdk",
			},
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rule.Specificity(); got != tt.want {
				t.Errorf("Specificity() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestBuildGateImageRule_Matches tests match logic.
func TestBuildGateImageRule_Matches(t *testing.T) {
	tests := []struct {
		name string
		rule BuildGateImageRule
		exp  StackExpectation
		want bool
	}{
		{
			name: "exact match",
			rule: BuildGateImageRule{
				Stack: StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			},
			exp:  StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			want: true,
		},
		{
			name: "tool-agnostic matches maven",
			rule: BuildGateImageRule{
				Stack: StackExpectation{Language: "java", Release: "17"},
			},
			exp:  StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			want: true,
		},
		{
			name: "tool-agnostic matches gradle",
			rule: BuildGateImageRule{
				Stack: StackExpectation{Language: "java", Release: "17"},
			},
			exp:  StackExpectation{Language: "java", Tool: "gradle", Release: "17"},
			want: true,
		},
		{
			name: "tool-agnostic matches empty tool",
			rule: BuildGateImageRule{
				Stack: StackExpectation{Language: "java", Release: "17"},
			},
			exp:  StackExpectation{Language: "java", Release: "17"},
			want: true,
		},
		{
			name: "language mismatch",
			rule: BuildGateImageRule{
				Stack: StackExpectation{Language: "java", Release: "17"},
			},
			exp:  StackExpectation{Language: "go", Release: "17"},
			want: false,
		},
		{
			name: "release mismatch",
			rule: BuildGateImageRule{
				Stack: StackExpectation{Language: "java", Release: "17"},
			},
			exp:  StackExpectation{Language: "java", Release: "11"},
			want: false,
		},
		{
			name: "tool mismatch when rule requires tool",
			rule: BuildGateImageRule{
				Stack: StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			},
			exp:  StackExpectation{Language: "java", Tool: "gradle", Release: "17"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rule.Matches(tt.exp); got != tt.want {
				t.Errorf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestBuildGateImageRule_SelectorKey tests unique key generation.
func TestBuildGateImageRule_SelectorKey(t *testing.T) {
	tests := []struct {
		name string
		rule BuildGateImageRule
		want string
	}{
		{
			name: "tool-specific",
			rule: BuildGateImageRule{
				Stack: StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			},
			want: "java:17:maven",
		},
		{
			name: "tool-agnostic uses wildcard",
			rule: BuildGateImageRule{
				Stack: StackExpectation{Language: "java", Release: "17"},
			},
			want: "java:17:*",
		},
		{
			name: "different release",
			rule: BuildGateImageRule{
				Stack: StackExpectation{Language: "java", Tool: "maven", Release: "11"},
			},
			want: "java:11:maven",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rule.SelectorKey(); got != tt.want {
				t.Errorf("SelectorKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBuildGateImageMapping_Validate tests validation rules.
func TestBuildGateImageMapping_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mapping BuildGateImageMapping
		wantErr string
	}{
		{
			name: "valid mapping",
			mapping: BuildGateImageMapping{
				Images: []BuildGateImageRule{
					{Stack: StackExpectation{Language: "java", Release: "17", Tool: "maven"}, Image: "maven:3-eclipse-temurin-17"},
					{Stack: StackExpectation{Language: "java", Release: "17"}, Image: "eclipse-temurin:17-jdk"},
				},
			},
			wantErr: "",
		},
		{
			name: "empty mapping is valid",
			mapping: BuildGateImageMapping{
				Images: []BuildGateImageRule{},
			},
			wantErr: "",
		},
		{
			name: "missing language",
			mapping: BuildGateImageMapping{
				Images: []BuildGateImageRule{
					{Stack: StackExpectation{Release: "17"}, Image: "test:latest"},
				},
			},
			wantErr: "build_gate.images[0].stack.language: required",
		},
		{
			name: "missing release",
			mapping: BuildGateImageMapping{
				Images: []BuildGateImageRule{
					{Stack: StackExpectation{Language: "java"}, Image: "test:latest"},
				},
			},
			wantErr: "build_gate.images[0].stack.release: required",
		},
		{
			name: "missing image",
			mapping: BuildGateImageMapping{
				Images: []BuildGateImageRule{
					{Stack: StackExpectation{Language: "java", Release: "17"}},
				},
			},
			wantErr: "build_gate.images[0].image: required",
		},
		{
			name: "duplicate selectors",
			mapping: BuildGateImageMapping{
				Images: []BuildGateImageRule{
					{Stack: StackExpectation{Language: "java", Release: "17"}, Image: "image1:latest"},
					{Stack: StackExpectation{Language: "java", Release: "17"}, Image: "image2:latest"},
				},
			},
			wantErr: `build_gate.images[1]: duplicate selector "java:17:*"`,
		},
		{
			name: "whitespace-only language",
			mapping: BuildGateImageMapping{
				Images: []BuildGateImageRule{
					{Stack: StackExpectation{Language: "   ", Release: "17"}, Image: "test:latest"},
				},
			},
			wantErr: "build_gate.images[0].stack.language: required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mapping.Validate("build_gate.images")
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected validation error")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestBuildGateImageRule_ParseRoundTrip tests parse → wire → parse consistency.
func TestBuildGateImageRule_ParseRoundTrip(t *testing.T) {
	original := &ModsSpec{
		Steps: []ModStep{{
			Image: JobImage{Universal: "docker.io/user/mod:latest"},
		}},
		BuildGate: &BuildGateConfig{
			Enabled: true,
			Images: []BuildGateImageRule{
				{Stack: StackExpectation{Language: "java", Release: "17", Tool: "maven"}, Image: "maven:3-eclipse-temurin-17"},
				{Stack: StackExpectation{Language: "java", Release: "17"}, Image: "eclipse-temurin:17-jdk"},
				{Stack: StackExpectation{Language: "java", Release: "11", Tool: "gradle"}, Image: "gradle:8.8-jdk11"},
			},
		},
	}

	// Convert to map.
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
	if parsed.BuildGate == nil {
		t.Fatal("build_gate is nil after round-trip")
	}

	if len(parsed.BuildGate.Images) != len(original.BuildGate.Images) {
		t.Fatalf("images count mismatch: got %d, want %d",
			len(parsed.BuildGate.Images), len(original.BuildGate.Images))
	}

	// Verify specific values.
	for i, orig := range original.BuildGate.Images {
		got := parsed.BuildGate.Images[i]
		if got.Stack.Language != orig.Stack.Language {
			t.Errorf("images[%d].stack.language = %q, want %q", i, got.Stack.Language, orig.Stack.Language)
		}
		if got.Stack.Release != orig.Stack.Release {
			t.Errorf("images[%d].stack.release = %q, want %q", i, got.Stack.Release, orig.Stack.Release)
		}
		if got.Stack.Tool != orig.Stack.Tool {
			t.Errorf("images[%d].stack.tool = %q, want %q", i, got.Stack.Tool, orig.Stack.Tool)
		}
		if got.Image != orig.Image {
			t.Errorf("images[%d].image = %q, want %q", i, got.Image, orig.Image)
		}
	}
}

// TestParseModsSpecJSON_BuildGateImages tests parsing build_gate.images.
func TestParseModsSpecJSON_BuildGateImages(t *testing.T) {
	input := `{
		"steps": [{"image": "test:latest"}],
		"build_gate": {
			"enabled": true,
			"images": [
				{
					"stack": {"language": "java", "release": "17", "tool": "maven"},
					"image": "maven:3-eclipse-temurin-17"
				},
				{
					"stack": {"language": "java", "release": "17"},
					"image": "eclipse-temurin:17-jdk"
				}
			]
		}
	}`

	spec, err := ParseModsSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	if spec.BuildGate == nil {
		t.Fatal("build_gate is nil")
	}

	if len(spec.BuildGate.Images) != 2 {
		t.Fatalf("len(images) = %d, want 2", len(spec.BuildGate.Images))
	}

	// Verify first rule (tool-specific).
	rule0 := spec.BuildGate.Images[0]
	if rule0.Stack.Language != "java" {
		t.Errorf("images[0].stack.language = %q, want %q", rule0.Stack.Language, "java")
	}
	if rule0.Stack.Release != "17" {
		t.Errorf("images[0].stack.release = %q, want %q", rule0.Stack.Release, "17")
	}
	if rule0.Stack.Tool != "maven" {
		t.Errorf("images[0].stack.tool = %q, want %q", rule0.Stack.Tool, "maven")
	}
	if rule0.Image != "maven:3-eclipse-temurin-17" {
		t.Errorf("images[0].image = %q, want %q", rule0.Image, "maven:3-eclipse-temurin-17")
	}

	// Verify second rule (tool-agnostic).
	rule1 := spec.BuildGate.Images[1]
	if rule1.Stack.Tool != "" {
		t.Errorf("images[1].stack.tool = %q, want empty", rule1.Stack.Tool)
	}
}

// TestParseModsSpecJSON_BuildGateImages_NumericRelease tests numeric release handling.
func TestParseModsSpecJSON_BuildGateImages_NumericRelease(t *testing.T) {
	input := `{
		"steps": [{"image": "test:latest"}],
		"build_gate": {
			"enabled": true,
			"images": [
				{
					"stack": {"language": "java", "release": 17},
					"image": "eclipse-temurin:17-jdk"
				},
				{
					"stack": {"language": "python", "release": 3.9},
					"image": "python:3.9"
				}
			]
		}
	}`

	spec, err := ParseModsSpecJSON([]byte(input))
	if err != nil {
		t.Fatalf("ParseModsSpecJSON failed: %v", err)
	}

	if spec.BuildGate.Images[0].Stack.Release != "17" {
		t.Errorf("images[0].stack.release = %q, want %q", spec.BuildGate.Images[0].Stack.Release, "17")
	}
	if spec.BuildGate.Images[1].Stack.Release != "3.9" {
		t.Errorf("images[1].stack.release = %q, want %q", spec.BuildGate.Images[1].Stack.Release, "3.9")
	}
}

// TestModsSpec_Validate_BuildGateImages tests validation integration.
func TestModsSpec_Validate_BuildGateImages(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{
			name: "valid images",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"images": [
						{"stack": {"language": "java", "release": "17"}, "image": "test:17"}
					]
				}
			}`,
			wantErr: "",
		},
		{
			name: "missing language",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"images": [
						{"stack": {"release": "17"}, "image": "test:17"}
					]
				}
			}`,
			wantErr: "build_gate.images[0].stack.language: required",
		},
		{
			name: "missing release",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"images": [
						{"stack": {"language": "java"}, "image": "test:latest"}
					]
				}
			}`,
			wantErr: "build_gate.images[0].stack.release: required",
		},
		{
			name: "duplicate selectors",
			input: `{
				"steps": [{"image": "test:latest"}],
				"build_gate": {
					"images": [
						{"stack": {"language": "java", "release": "17"}, "image": "image1:latest"},
						{"stack": {"language": "java", "release": "17"}, "image": "image2:latest"}
					]
				}
			}`,
			wantErr: `build_gate.images[1]: duplicate selector "java:17:*"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseModsSpecJSON([]byte(tt.input))
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected validation error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}
