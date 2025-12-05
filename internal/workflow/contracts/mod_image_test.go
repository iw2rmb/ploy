package contracts

import (
	"strings"
	"testing"
)

// TestModImage_ResolveImage_Universal verifies that universal (string) images
// are returned regardless of the stack parameter.
func TestModImage_ResolveImage_Universal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		image ModImage
		stack ModStack
		want  string
	}{
		{
			name:  "universal image with java-maven stack",
			image: ModImage{Universal: "docker.io/user/mods-orw:latest"},
			stack: ModStackJavaMaven,
			want:  "docker.io/user/mods-orw:latest",
		},
		{
			name:  "universal image with java-gradle stack",
			image: ModImage{Universal: "docker.io/user/mods-orw:latest"},
			stack: ModStackJavaGradle,
			want:  "docker.io/user/mods-orw:latest",
		},
		{
			name:  "universal image with unknown stack",
			image: ModImage{Universal: "docker.io/user/mods-orw:latest"},
			stack: ModStackUnknown,
			want:  "docker.io/user/mods-orw:latest",
		},
		{
			name:  "universal image with empty stack (defaults to unknown)",
			image: ModImage{Universal: "docker.io/user/mods-orw:latest"},
			stack: "",
			want:  "docker.io/user/mods-orw:latest",
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

// TestModImage_ResolveImage_StackSpecific verifies that stack-specific maps
// resolve to the correct image based on stack matching and default fallback.
func TestModImage_ResolveImage_StackSpecific(t *testing.T) {
	t.Parallel()

	// Stack map with exact keys and default.
	stackMap := ModImage{
		ByStack: map[ModStack]string{
			ModStackDefault:    "docker.io/user/mods-orw:latest",
			ModStackJavaMaven:  "docker.io/user/mods-orw-maven:latest",
			ModStackJavaGradle: "docker.io/user/mods-orw-gradle:latest",
		},
	}

	tests := []struct {
		name  string
		image ModImage
		stack ModStack
		want  string
	}{
		{
			name:  "exact match java-maven",
			image: stackMap,
			stack: ModStackJavaMaven,
			want:  "docker.io/user/mods-orw-maven:latest",
		},
		{
			name:  "exact match java-gradle",
			image: stackMap,
			stack: ModStackJavaGradle,
			want:  "docker.io/user/mods-orw-gradle:latest",
		},
		{
			name:  "fallback to default for java stack",
			image: stackMap,
			stack: ModStackJava,
			want:  "docker.io/user/mods-orw:latest",
		},
		{
			name:  "fallback to default for unknown stack",
			image: stackMap,
			stack: ModStackUnknown,
			want:  "docker.io/user/mods-orw:latest",
		},
		{
			name:  "fallback to default for empty stack",
			image: stackMap,
			stack: "",
			want:  "docker.io/user/mods-orw:latest",
		},
		{
			name:  "fallback to default for custom stack",
			image: stackMap,
			stack: ModStack("python-pip"),
			want:  "docker.io/user/mods-orw:latest",
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

// TestModImage_ResolveImage_NoDefault verifies that resolution fails with an
// actionable error when no exact match exists and no default is provided.
func TestModImage_ResolveImage_NoDefault(t *testing.T) {
	t.Parallel()

	// Stack map without default key.
	stackMap := ModImage{
		ByStack: map[ModStack]string{
			ModStackJavaMaven: "docker.io/user/mods-orw-maven:latest",
		},
	}

	tests := []struct {
		name        string
		stack       ModStack
		wantErrPart string
	}{
		{
			name:        "missing java-gradle with no default",
			stack:       ModStackJavaGradle,
			wantErrPart: "no image specified for stack",
		},
		{
			name:        "missing unknown with no default",
			stack:       ModStackUnknown,
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

// TestModImage_ResolveImage_Empty verifies that resolving an empty ModImage
// returns an error.
func TestModImage_ResolveImage_Empty(t *testing.T) {
	t.Parallel()

	empty := ModImage{}
	_, err := empty.ResolveImage(ModStackJavaMaven)
	if err == nil {
		t.Fatal("expected error for empty ModImage")
	}
	if !strings.Contains(err.Error(), "image not specified") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestParseModImage_String verifies parsing of string (universal) images.
func TestParseModImage_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input any
		want  string
	}{
		{
			name:  "simple string",
			input: "docker.io/user/mod:latest",
			want:  "docker.io/user/mod:latest",
		},
		{
			name:  "string with whitespace",
			input: "  docker.io/user/mod:v1  ",
			want:  "docker.io/user/mod:v1",
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
			got, err := ParseModImage(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Universal != tt.want {
				t.Errorf("ParseModImage(%v).Universal = %q, want %q", tt.input, got.Universal, tt.want)
			}
			if len(got.ByStack) != 0 {
				t.Errorf("ParseModImage(%v).ByStack should be empty, got %v", tt.input, got.ByStack)
			}
		})
	}
}

// TestParseModImage_Map verifies parsing of map (stack-specific) images.
func TestParseModImage_Map(t *testing.T) {
	t.Parallel()

	t.Run("map[string]any from JSON/YAML", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{
			"default":     "docker.io/user/mods-orw:latest",
			"java-maven":  "docker.io/user/mods-orw-maven:latest",
			"java-gradle": "docker.io/user/mods-orw-gradle:latest",
		}

		got, err := ParseModImage(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Universal != "" {
			t.Errorf("expected empty Universal, got %q", got.Universal)
		}
		if len(got.ByStack) != 3 {
			t.Errorf("expected 3 stack entries, got %d", len(got.ByStack))
		}
		if got.ByStack[ModStackDefault] != "docker.io/user/mods-orw:latest" {
			t.Errorf("default image mismatch: %q", got.ByStack[ModStackDefault])
		}
		if got.ByStack[ModStackJavaMaven] != "docker.io/user/mods-orw-maven:latest" {
			t.Errorf("java-maven image mismatch: %q", got.ByStack[ModStackJavaMaven])
		}
		if got.ByStack[ModStackJavaGradle] != "docker.io/user/mods-orw-gradle:latest" {
			t.Errorf("java-gradle image mismatch: %q", got.ByStack[ModStackJavaGradle])
		}
	})

	t.Run("map[string]string typed", func(t *testing.T) {
		t.Parallel()
		input := map[string]string{
			"default":    "img:default",
			"java-maven": "img:maven",
		}

		got, err := ParseModImage(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.ByStack) != 2 {
			t.Errorf("expected 2 stack entries, got %d", len(got.ByStack))
		}
		if got.ByStack[ModStackDefault] != "img:default" {
			t.Errorf("default image mismatch: %q", got.ByStack[ModStackDefault])
		}
	})

	t.Run("empty map", func(t *testing.T) {
		t.Parallel()
		input := map[string]any{}

		got, err := ParseModImage(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got.ByStack) != 0 {
			t.Errorf("expected empty ByStack, got %v", got.ByStack)
		}
	})
}

// TestParseModImage_Nil verifies that nil input returns empty ModImage.
func TestParseModImage_Nil(t *testing.T) {
	t.Parallel()

	got, err := ParseModImage(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsEmpty() {
		t.Errorf("expected empty ModImage for nil input, got %v", got)
	}
}

// TestParseModImage_InvalidType verifies error handling for invalid types.
func TestParseModImage_InvalidType(t *testing.T) {
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
			_, err := ParseModImage(tt.input)
			if err == nil {
				t.Fatal("expected error for invalid type")
			}
			if !strings.Contains(err.Error(), "expected string or map") {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestParseModImage_MapWithInvalidValue verifies error handling for maps
// containing non-string values.
func TestParseModImage_MapWithInvalidValue(t *testing.T) {
	t.Parallel()

	input := map[string]any{
		"default":    "valid:image",
		"java-maven": 123, // Invalid: not a string.
	}

	_, err := ParseModImage(input)
	if err == nil {
		t.Fatal("expected error for map with non-string value")
	}
	if !strings.Contains(err.Error(), "expected string") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestModImage_IsEmpty verifies the IsEmpty method.
func TestModImage_IsEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		image ModImage
		want  bool
	}{
		{name: "empty", image: ModImage{}, want: true},
		{name: "universal", image: ModImage{Universal: "img:v1"}, want: false},
		{name: "stack map", image: ModImage{ByStack: map[ModStack]string{"default": "img:v1"}}, want: false},
		{name: "empty stack map", image: ModImage{ByStack: map[ModStack]string{}}, want: true},
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

// TestModImage_IsUniversal verifies the IsUniversal method.
func TestModImage_IsUniversal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		image ModImage
		want  bool
	}{
		{name: "universal only", image: ModImage{Universal: "img:v1"}, want: true},
		{name: "empty", image: ModImage{}, want: false},
		{name: "stack map only", image: ModImage{ByStack: map[ModStack]string{"default": "img:v1"}}, want: false},
		// When both are set, ByStack takes precedence (not universal).
		{name: "both set", image: ModImage{Universal: "img:v1", ByStack: map[ModStack]string{"default": "img:v2"}}, want: false},
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

// TestModImage_IsStackSpecific verifies the IsStackSpecific method.
func TestModImage_IsStackSpecific(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		image ModImage
		want  bool
	}{
		{name: "stack map", image: ModImage{ByStack: map[ModStack]string{"default": "img:v1"}}, want: true},
		{name: "empty", image: ModImage{}, want: false},
		{name: "universal only", image: ModImage{Universal: "img:v1"}, want: false},
		{name: "empty stack map", image: ModImage{ByStack: map[ModStack]string{}}, want: false},
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

// TestModImage_String verifies the String() method for debugging output.
func TestModImage_String(t *testing.T) {
	t.Parallel()

	t.Run("universal", func(t *testing.T) {
		t.Parallel()
		img := ModImage{Universal: "docker.io/user/mod:latest"}
		got := img.String()
		if got != "docker.io/user/mod:latest" {
			t.Errorf("String() = %q, want universal image", got)
		}
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		img := ModImage{}
		got := img.String()
		if got != "<empty>" {
			t.Errorf("String() = %q, want <empty>", got)
		}
	})

	t.Run("stack map", func(t *testing.T) {
		t.Parallel()
		img := ModImage{ByStack: map[ModStack]string{
			"default": "img:default",
		}}
		got := img.String()
		if !strings.Contains(got, "default=img:default") {
			t.Errorf("String() = %q, expected to contain stack entries", got)
		}
	})
}

// TestToolToModStack verifies conversion from Build Gate tool names to ModStack.
func TestToolToModStack(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tool string
		want ModStack
	}{
		{
			name: "maven lowercase",
			tool: "maven",
			want: ModStackJavaMaven,
		},
		{
			name: "maven mixed case",
			tool: "Maven",
			want: ModStackJavaMaven,
		},
		{
			name: "maven with whitespace",
			tool: "  maven  ",
			want: ModStackJavaMaven,
		},
		{
			name: "gradle lowercase",
			tool: "gradle",
			want: ModStackJavaGradle,
		},
		{
			name: "gradle mixed case",
			tool: "GRADLE",
			want: ModStackJavaGradle,
		},
		{
			name: "java lowercase",
			tool: "java",
			want: ModStackJava,
		},
		{
			name: "java mixed case",
			tool: "Java",
			want: ModStackJava,
		},
		{
			name: "empty string",
			tool: "",
			want: ModStackUnknown,
		},
		{
			name: "whitespace only",
			tool: "   ",
			want: ModStackUnknown,
		},
		{
			name: "unknown tool",
			tool: "bazel",
			want: ModStackUnknown,
		},
		{
			name: "none tool (gate skipped)",
			tool: "none",
			want: ModStackUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ToolToModStack(tt.tool)
			if got != tt.want {
				t.Errorf("ToolToModStack(%q) = %q, want %q", tt.tool, got, tt.want)
			}
		})
	}
}
