package contracts

import (
	"strings"
	"testing"
)

// TestJobImage_ResolveImage_Universal verifies that universal (string) images
// are returned regardless of the stack parameter.
func TestJobImage_ResolveImage_Universal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		image JobImage
		stack MigStack
		want  string
	}{
		{
			name:  "universal image with java-maven stack",
			image: JobImage{Universal: "docker.io/user/migs-orw:latest"},
			stack: MigStackJavaMaven,
			want:  "docker.io/user/migs-orw:latest",
		},
		{
			name:  "universal image with java-gradle stack",
			image: JobImage{Universal: "docker.io/user/migs-orw:latest"},
			stack: MigStackJavaGradle,
			want:  "docker.io/user/migs-orw:latest",
		},
		{
			name:  "universal image with unknown stack",
			image: JobImage{Universal: "docker.io/user/migs-orw:latest"},
			stack: MigStackUnknown,
			want:  "docker.io/user/migs-orw:latest",
		},
		{
			name:  "universal image with empty stack (defaults to unknown)",
			image: JobImage{Universal: "docker.io/user/migs-orw:latest"},
			stack: "",
			want:  "docker.io/user/migs-orw:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.image.ResolveImage(tt.stack)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ResolveImage(%q) = %q, want %q", tt.stack, got, tt.want)
			}
		})
	}
}

// TestJobImage_ResolveImage_StackSpecific verifies that stack-specific maps
// resolve to the correct image based on stack matching and default fallback.
func TestJobImage_ResolveImage_StackSpecific(t *testing.T) {
	t.Parallel()

	// Stack map with exact keys and default.
	stackMap := JobImage{
		ByStack: map[MigStack]string{
			MigStackDefault:    "docker.io/user/migs-orw:latest",
			MigStackJavaMaven:  "docker.io/user/orw-cli:latest",
			MigStackJavaGradle: "docker.io/user/orw-cli:latest",
		},
	}

	tests := []struct {
		name  string
		image JobImage
		stack MigStack
		want  string
	}{
		{
			name:  "exact match java-maven",
			image: stackMap,
			stack: MigStackJavaMaven,
			want:  "docker.io/user/orw-cli:latest",
		},
		{
			name:  "exact match java-gradle",
			image: stackMap,
			stack: MigStackJavaGradle,
			want:  "docker.io/user/orw-cli:latest",
		},
		{
			name:  "fallback to default for java stack",
			image: stackMap,
			stack: MigStackJava,
			want:  "docker.io/user/migs-orw:latest",
		},
		{
			name:  "fallback to default for unknown stack",
			image: stackMap,
			stack: MigStackUnknown,
			want:  "docker.io/user/migs-orw:latest",
		},
		{
			name:  "fallback to default for empty stack",
			image: stackMap,
			stack: "",
			want:  "docker.io/user/migs-orw:latest",
		},
		{
			name:  "fallback to default for custom stack",
			image: stackMap,
			stack: MigStack("python-pip"),
			want:  "docker.io/user/migs-orw:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.image.ResolveImage(tt.stack)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ResolveImage(%q) = %q, want %q", tt.stack, got, tt.want)
			}
		})
	}
}

// TestJobImage_ResolveImage_NoDefault verifies that resolution fails with an
// actionable error when no exact match exists and no default is provided.
func TestJobImage_ResolveImage_NoDefault(t *testing.T) {
	t.Parallel()

	// Stack map without default key.
	stackMap := JobImage{
		ByStack: map[MigStack]string{
			MigStackJavaMaven: "docker.io/user/orw-cli:latest",
		},
	}

	tests := []struct {
		name        string
		stack       MigStack
		wantErrPart string
	}{
		{
			name:        "missing java-gradle with no default",
			stack:       MigStackJavaGradle,
			wantErrPart: "no image specified for stack",
		},
		{
			name:        "missing unknown with no default",
			stack:       MigStackUnknown,
			wantErrPart: "no default provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := stackMap.ResolveImage(tt.stack)
			if err == nil {
				t.Fatalf("expected error, got image=%q", got)
			}
			if !strings.Contains(err.Error(), tt.wantErrPart) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrPart)
			}
		})
	}
}

// TestJobImage_ResolveImage_Empty verifies that resolving an empty JobImage
// returns an error.
func TestJobImage_ResolveImage_Empty(t *testing.T) {
	t.Parallel()

	empty := JobImage{}
	_, err := empty.ResolveImage(MigStackJavaMaven)
	if err == nil {
		t.Fatal("expected error for empty JobImage")
	}
	if !strings.Contains(err.Error(), "image not specified") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestParseJobImage_String verifies parsing of string (universal) images.
func TestParseJobImage_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input any
		want  string
	}{
		{
			name:  "simple string",
			input: "docker.io/user/mig:latest",
			want:  "docker.io/user/mig:latest",
		},
		{
			name:  "string with whitespace",
			input: "  docker.io/user/mig:v1  ",
			want:  "docker.io/user/mig:v1",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseJobImage(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Universal != tt.want {
				t.Errorf("ParseJobImage(%v).Universal = %q, want %q", tt.input, got.Universal, tt.want)
			}
			if len(got.ByStack) != 0 {
				t.Errorf("ParseJobImage(%v).ByStack should be empty, got %v", tt.input, got.ByStack)
			}
		})
	}
}

// TestParseJobImage_Map verifies parsing of map (stack-specific) images.
func TestParseJobImage_Map(t *testing.T) {
	t.Parallel()

	t.Run("map[string]any from JSON/YAML", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{
			"default":     "docker.io/user/migs-orw:latest",
			"java-maven":  "docker.io/user/orw-cli:latest",
			"java-gradle": "docker.io/user/orw-cli:latest",
		}

		got, err := ParseJobImage(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Universal != "" {
			t.Errorf("expected empty Universal, got %q", got.Universal)
		}
		if len(got.ByStack) != 3 {
			t.Errorf("expected 3 stack entries, got %d", len(got.ByStack))
		}
		if got.ByStack[MigStackDefault] != "docker.io/user/migs-orw:latest" {
			t.Errorf("default image mismatch: %q", got.ByStack[MigStackDefault])
		}
		if got.ByStack[MigStackJavaMaven] != "docker.io/user/orw-cli:latest" {
			t.Errorf("java-maven image mismatch: %q", got.ByStack[MigStackJavaMaven])
		}
		if got.ByStack[MigStackJavaGradle] != "docker.io/user/orw-cli:latest" {
			t.Errorf("java-gradle image mismatch: %q", got.ByStack[MigStackJavaGradle])
		}
	})

	t.Run("map[string]string typed", func(t *testing.T) {
		t.Parallel()
		input := map[string]string{
			"default":    "img:default",
			"java-maven": "img:maven",
		}

		got, err := ParseJobImage(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.ByStack) != 2 {
			t.Errorf("expected 2 stack entries, got %d", len(got.ByStack))
		}
		if got.ByStack[MigStackDefault] != "img:default" {
			t.Errorf("default image mismatch: %q", got.ByStack[MigStackDefault])
		}
	})

	t.Run("empty map", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{}

		got, err := ParseJobImage(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.ByStack) != 0 {
			t.Errorf("expected empty ByStack, got %v", got.ByStack)
		}
	})
}

// TestParseJobImage_Nil verifies that nil input returns empty JobImage.
func TestParseJobImage_Nil(t *testing.T) {
	t.Parallel()

	got, err := ParseJobImage(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsEmpty() {
		t.Errorf("expected empty JobImage for nil input, got %v", got)
	}
}

// TestParseJobImage_InvalidType verifies error handling for invalid types.
func TestParseJobImage_InvalidType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input any
	}{
		{name: "int", input: 42},
		{name: "bool", input: true},
		{name: "slice", input: []string{"a", "b"}},
		{name: "float64", input: 3.14},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseJobImage(tt.input)
			if err == nil {
				t.Fatal("expected error for invalid type")
			}
			if !strings.Contains(err.Error(), "expected string or map") {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestParseJobImage_MapWithInvalidValue verifies error handling for maps
// containing non-string values.
func TestParseJobImage_MapWithInvalidValue(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"default":    "valid:image",
		"java-maven": 123, // Invalid: not a string.
	}

	_, err := ParseJobImage(input)
	if err == nil {
		t.Fatal("expected error for map with non-string value")
	}
	if !strings.Contains(err.Error(), "expected string") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestJobImage_IsEmpty verifies the IsEmpty method.
func TestJobImage_IsEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		image JobImage
		want  bool
	}{
		{name: "empty", image: JobImage{}, want: true},
		{name: "universal", image: JobImage{Universal: "img:v1"}, want: false},
		{name: "stack map", image: JobImage{ByStack: map[MigStack]string{"default": "img:v1"}}, want: false},
		{name: "empty stack map", image: JobImage{ByStack: map[MigStack]string{}}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.image.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestJobImage_IsUniversal verifies the IsUniversal method.
func TestJobImage_IsUniversal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		image JobImage
		want  bool
	}{
		{name: "universal only", image: JobImage{Universal: "img:v1"}, want: true},
		{name: "empty", image: JobImage{}, want: false},
		{name: "stack map only", image: JobImage{ByStack: map[MigStack]string{"default": "img:v1"}}, want: false},
		// When both are set, ByStack takes precedence (not universal).
		{name: "both set", image: JobImage{Universal: "img:v1", ByStack: map[MigStack]string{"default": "img:v2"}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.image.IsUniversal(); got != tt.want {
				t.Errorf("IsUniversal() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestJobImage_IsStackSpecific verifies the IsStackSpecific method.
func TestJobImage_IsStackSpecific(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		image JobImage
		want  bool
	}{
		{name: "stack map", image: JobImage{ByStack: map[MigStack]string{"default": "img:v1"}}, want: true},
		{name: "empty", image: JobImage{}, want: false},
		{name: "universal only", image: JobImage{Universal: "img:v1"}, want: false},
		{name: "empty stack map", image: JobImage{ByStack: map[MigStack]string{}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.image.IsStackSpecific(); got != tt.want {
				t.Errorf("IsStackSpecific() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestJobImage_String verifies the String() method for debugging output.
func TestJobImage_String(t *testing.T) {
	t.Parallel()

	t.Run("universal", func(t *testing.T) {
		t.Parallel()
		img := JobImage{Universal: "docker.io/user/mig:latest"}
		got := img.String()
		if got != "docker.io/user/mig:latest" {
			t.Errorf("String() = %q, want universal image", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		img := JobImage{}
		got := img.String()
		if got != "<empty>" {
			t.Errorf("String() = %q, want <empty>", got)
		}
	})

	t.Run("stack map", func(t *testing.T) {
		t.Parallel()
		img := JobImage{ByStack: map[MigStack]string{
			"default": "img:default",
		}}
		got := img.String()
		if !strings.Contains(got, "default=img:default") {
			t.Errorf("String() = %q, expected to contain stack entries", got)
		}
	})
}

// TestToolToModStack verifies conversion from Build Gate tool names to MigStack.
func TestToolToModStack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tool string
		want MigStack
	}{
		{
			name: "maven lowercase",
			tool: "maven",
			want: MigStackJavaMaven,
		},
		{
			name: "maven mixed case",
			tool: "Maven",
			want: MigStackJavaMaven,
		},
		{
			name: "maven with whitespace",
			tool: "  maven  ",
			want: MigStackJavaMaven,
		},
		{
			name: "gradle lowercase",
			tool: "gradle",
			want: MigStackJavaGradle,
		},
		{
			name: "gradle mixed case",
			tool: "GRADLE",
			want: MigStackJavaGradle,
		},
		{
			name: "java lowercase",
			tool: "java",
			want: MigStackJava,
		},
		{
			name: "java mixed case",
			tool: "Java",
			want: MigStackJava,
		},
		{
			name: "empty string",
			tool: "",
			want: MigStackUnknown,
		},
		{
			name: "whitespace only",
			tool: "   ",
			want: MigStackUnknown,
		},
		{
			name: "unknown tool",
			tool: "bazel",
			want: MigStackUnknown,
		},
		{
			name: "none tool (gate skipped)",
			tool: "none",
			want: MigStackUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ToolToMigStack(tt.tool)
			if got != tt.want {
				t.Errorf("ToolToMigStack(%q) = %q, want %q", tt.tool, got, tt.want)
			}
		})
	}
}
