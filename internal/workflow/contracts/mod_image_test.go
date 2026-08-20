package contracts

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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
			image: JobImage{Universal: "ghcr.io/iw2rmb/ploy/migs-orw:latest"},
			stack: MigStackJavaMaven,
			want:  "ghcr.io/iw2rmb/ploy/migs-orw:latest",
		},
		{
			name:  "universal image with java-gradle stack",
			image: JobImage{Universal: "ghcr.io/iw2rmb/ploy/migs-orw:latest"},
			stack: MigStackJavaGradle,
			want:  "ghcr.io/iw2rmb/ploy/migs-orw:latest",
		},
		{
			name:  "universal image with unknown stack",
			image: JobImage{Universal: "ghcr.io/iw2rmb/ploy/migs-orw:latest"},
			stack: MigStackUnknown,
			want:  "ghcr.io/iw2rmb/ploy/migs-orw:latest",
		},
		{
			name:  "universal image with empty stack (defaults to unknown)",
			image: JobImage{Universal: "ghcr.io/iw2rmb/ploy/migs-orw:latest"},
			stack: "",
			want:  "ghcr.io/iw2rmb/ploy/migs-orw:latest",
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
			MigStackDefault:    "ghcr.io/iw2rmb/ploy/migs-orw:latest",
			MigStackJavaMaven:  "ghcr.io/iw2rmb/ploy/orw-cli:latest",
			MigStackJavaGradle: "ghcr.io/iw2rmb/ploy/orw-cli:latest",
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
			want:  "ghcr.io/iw2rmb/ploy/orw-cli:latest",
		},
		{
			name:  "exact match java-gradle",
			image: stackMap,
			stack: MigStackJavaGradle,
			want:  "ghcr.io/iw2rmb/ploy/orw-cli:latest",
		},
		{
			name:  "fallback to default for java stack",
			image: stackMap,
			stack: MigStackJava,
			want:  "ghcr.io/iw2rmb/ploy/migs-orw:latest",
		},
		{
			name:  "fallback to default for unknown stack",
			image: stackMap,
			stack: MigStackUnknown,
			want:  "ghcr.io/iw2rmb/ploy/migs-orw:latest",
		},
		{
			name:  "fallback to default for empty stack",
			image: stackMap,
			stack: "",
			want:  "ghcr.io/iw2rmb/ploy/migs-orw:latest",
		},
		{
			name:  "fallback to default for custom stack",
			image: stackMap,
			stack: MigStack("python-pip"),
			want:  "ghcr.io/iw2rmb/ploy/migs-orw:latest",
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
			MigStackJavaMaven: "ghcr.io/iw2rmb/ploy/orw-cli:latest",
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

func TestJobImage_UnmarshalJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		wire       string
		universal  string
		stackImage string
		wantEmpty  bool
		wantErr    bool
	}{
		{name: "universal", wire: `" ghcr.io/iw2rmb/ploy/mig:latest "`, universal: "ghcr.io/iw2rmb/ploy/mig:latest"},
		{name: "stack map", wire: `{"default":" img:default "}`, stackImage: "img:default"},
		{name: "null", wire: `null`, wantEmpty: true},
		{name: "invalid type", wire: `42`, wantErr: true},
		{name: "invalid map value", wire: `{"default":42}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var image JobImage
			err := json.Unmarshal([]byte(tt.wire), &image)
			if (err != nil) != tt.wantErr {
				t.Fatalf("json.Unmarshal() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if image.Universal != tt.universal {
				t.Fatalf("Universal = %q, want %q", image.Universal, tt.universal)
			}
			if image.ByStack[MigStackDefault] != tt.stackImage {
				t.Fatalf("default image = %q, want %q", image.ByStack[MigStackDefault], tt.stackImage)
			}
			if tt.wantEmpty && !image.IsEmpty() {
				t.Fatalf("image = %v, want empty", image)
			}
		})
	}
}

func TestJobImage_UnmarshalYAML_String(t *testing.T) {
	t.Parallel()

	var got struct {
		Image JobImage `yaml:"image"`
	}
	if err := yaml.Unmarshal([]byte("image: ghcr.io/iw2rmb/ploy/mig:latest\n"), &got); err != nil {
		t.Fatalf("yaml unmarshal: %v", err)
	}
	if got.Image.Universal != "ghcr.io/iw2rmb/ploy/mig:latest" {
		t.Fatalf("image.universal=%q, want universal image", got.Image.Universal)
	}
	if len(got.Image.ByStack) != 0 {
		t.Fatalf("image.by_stack=%v, want empty", got.Image.ByStack)
	}
}

func TestJobImage_UnmarshalYAML_Map(t *testing.T) {
	t.Parallel()

	var got struct {
		Image JobImage `yaml:"image"`
	}
	doc := `
image:
  default: ghcr.io/iw2rmb/ploy/mig:default
  java-gradle: ghcr.io/iw2rmb/ploy/mig:gradle
`
	if err := yaml.Unmarshal([]byte(doc), &got); err != nil {
		t.Fatalf("yaml unmarshal: %v", err)
	}
	if got.Image.Universal != "" {
		t.Fatalf("image.universal=%q, want empty", got.Image.Universal)
	}
	if got.Image.ByStack[MigStackDefault] != "ghcr.io/iw2rmb/ploy/mig:default" {
		t.Fatalf("default image=%q, want default image", got.Image.ByStack[MigStackDefault])
	}
	if got.Image.ByStack[MigStackJavaGradle] != "ghcr.io/iw2rmb/ploy/mig:gradle" {
		t.Fatalf("java-gradle image=%q, want gradle image", got.Image.ByStack[MigStackJavaGradle])
	}
}

func TestJobImageClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		image         JobImage
		wantEmpty     bool
		wantUniversal bool
		wantByStack   bool
	}{
		{name: "empty", image: JobImage{}, wantEmpty: true},
		{name: "empty stack map", image: JobImage{ByStack: map[MigStack]string{}}, wantEmpty: true},
		{name: "universal", image: JobImage{Universal: "img:v1"}, wantUniversal: true},
		{name: "stack map", image: JobImage{ByStack: map[MigStack]string{"default": "img:v1"}}, wantByStack: true},
		{name: "both prefer stack map", image: JobImage{Universal: "img:v1", ByStack: map[MigStack]string{"default": "img:v2"}}, wantByStack: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.image.IsEmpty(); got != tt.wantEmpty {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.wantEmpty)
			}
			if got := tt.image.IsUniversal(); got != tt.wantUniversal {
				t.Errorf("IsUniversal() = %v, want %v", got, tt.wantUniversal)
			}
			if got := tt.image.IsStackSpecific(); got != tt.wantByStack {
				t.Errorf("IsStackSpecific() = %v, want %v", got, tt.wantByStack)
			}
		})
	}
}

// TestJobImage_String verifies the String() method for debugging output.
func TestJobImage_String(t *testing.T) {
	t.Parallel()

	t.Run("universal", func(t *testing.T) {
		t.Parallel()
		img := JobImage{Universal: "ghcr.io/iw2rmb/ploy/mig:latest"}
		got := img.String()
		if got != "ghcr.io/iw2rmb/ploy/mig:latest" {
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
