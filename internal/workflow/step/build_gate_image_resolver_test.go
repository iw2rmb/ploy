package step

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func writeDummyProfile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir profile dir: %v", err)
	}
	const profile = "schema_version: 1\nrepo_id: default\nrunner_mode: simple\nstack:\n  language: java\n  tool: maven\ntargets:\n  active: build\n  build:\n    status: not_attempted\n    command: echo ok\n    env: {}\n  unit:\n    status: not_attempted\n    env: {}\n  all_tests:\n    status: not_attempted\n    env: {}\norchestration:\n  pre: []\n  post: []\n"
	if err := os.WriteFile(path, []byte(profile), 0o644); err != nil {
		t.Fatalf("write profile file: %v", err)
	}
}

// TestBuildGateImageResolver_Resolve tests resolution logic.
func TestBuildGateImageResolver_Resolve(t *testing.T) {
	rules := []contracts.BuildGateImageRule{
		{Stack: contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"}, Image: "maven:3-eclipse-temurin-17"},
		{Stack: contracts.StackExpectation{Language: "java", Release: "17", Tool: "gradle"}, Image: "gradle:8.8-jdk17"},
		{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "eclipse-temurin:17-jdk"},
		{Stack: contracts.StackExpectation{Language: "java", Release: "11"}, Image: "eclipse-temurin:11-jdk"},
	}

	resolver := &BuildGateImageResolver{rules: rules}

	tests := []struct {
		name    string
		exp     contracts.StackExpectation
		want    string
		wantErr string
	}{
		{
			name: "exact match maven",
			exp:  contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"},
			want: "maven:3-eclipse-temurin-17",
		},
		{
			name: "exact match gradle",
			exp:  contracts.StackExpectation{Language: "java", Release: "17", Tool: "gradle"},
			want: "gradle:8.8-jdk17",
		},
		{
			name: "tool-agnostic fallback (unknown tool)",
			exp:  contracts.StackExpectation{Language: "java", Release: "17", Tool: "ant"},
			want: "eclipse-temurin:17-jdk",
		},
		{
			name: "tool-agnostic fallback (no tool)",
			exp:  contracts.StackExpectation{Language: "java", Release: "17"},
			want: "eclipse-temurin:17-jdk",
		},
		{
			name: "different release",
			exp:  contracts.StackExpectation{Language: "java", Release: "11"},
			want: "eclipse-temurin:11-jdk",
		},
		{
			name:    "no match - wrong language",
			exp:     contracts.StackExpectation{Language: "go", Release: "1.21"},
			wantErr: "no image rule matches stack go:1.21:",
		},
		{
			name:    "no match - wrong release",
			exp:     contracts.StackExpectation{Language: "java", Release: "21"},
			wantErr: "no image rule matches stack java:21:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolver.Resolve(tt.exp)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBuildGateImageResolver_Precedence tests that higher precedence rules win.
func TestBuildGateImageResolver_Precedence(t *testing.T) {
	// Default file rules (lowest precedence).
	defaultRules := []contracts.BuildGateImageRule{
		{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "default:17"},
	}

	// Mig override rules (highest precedence).
	migRules := []contracts.BuildGateImageRule{
		{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "mig:17"},
	}

	tests := []struct {
		name         string
		defaultRules []contracts.BuildGateImageRule
		migRules     []contracts.BuildGateImageRule
		wantImage    string
	}{
		{
			name:         "mig overrides all",
			defaultRules: defaultRules,
			migRules:     migRules,
			wantImage:    "mig:17",
		},
		{
			name:         "default when no overrides",
			defaultRules: defaultRules,
			migRules:     nil,
			wantImage:    "default:17",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build merged rules manually (simulating NewBuildGateImageResolver without file loading).
			var rules []contracts.BuildGateImageRule
			rules = append(rules, tt.defaultRules...)
			rules = append(rules, tt.migRules...)

			resolver := &BuildGateImageResolver{rules: rules}
			got, err := resolver.Resolve(contracts.StackExpectation{Language: "java", Release: "17"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantImage {
				t.Errorf("Resolve() = %q, want %q", got, tt.wantImage)
			}
		})
	}
}

// TestBuildGateImageResolver_PrecedenceLastWins tests that the last rule at same specificity wins.
func TestBuildGateImageResolver_PrecedenceLastWins(t *testing.T) {
	// Two rules at same specificity with different images - last one should win.
	rules := []contracts.BuildGateImageRule{
		{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "image1:17"},
		{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "image2:17"},
	}

	resolver := &BuildGateImageResolver{rules: rules}
	got, err := resolver.Resolve(contracts.StackExpectation{Language: "java", Release: "17"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Last rule wins (higher precedence).
	if got != "image2:17" {
		t.Errorf("Resolve() = %q, want %q (last rule should win)", got, "image2:17")
	}
}

// TestBuildGateImageResolver_MissingFile tests error handling for missing file.
func TestBuildGateImageResolver_MissingFile(t *testing.T) {
	// When requireDefaultFile=true and file is missing, should error.
	_, err := NewBuildGateImageResolver("/nonexistent/path/stacks.yaml", nil, true)
	if err == nil {
		t.Fatal("expected error for missing file when requireDefaultFile=true")
	}
	if !strings.Contains(err.Error(), "required but not found") {
		t.Errorf("error = %q, want to contain 'required but not found'", err.Error())
	}

	// When requireDefaultFile=false and file is missing, should not error.
	resolver, err := NewBuildGateImageResolver("/nonexistent/path/stacks.yaml", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolver == nil {
		t.Fatal("resolver is nil")
	}
}

func TestBuildGateImageResolver_ExpandsRegistryPrefixFromEnv(t *testing.T) {
	t.Setenv(containerRegistryEnvKey, "192.0.2.25:5001/ploy")

	resolver, err := NewBuildGateImageResolver("", []contracts.BuildGateImageRule{
		{
			Stack: contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"},
			Image: "$PLOY_CONTAINER_REGISTRY/maven:3-eclipse-temurin-17",
		},
	}, false)
	if err != nil {
		t.Fatalf("NewBuildGateImageResolver() error: %v", err)
	}

	got, err := resolver.Resolve(contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"})
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	want := "192.0.2.25:5001/ploy/maven:3-eclipse-temurin-17"
	if got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}
}

func TestBuildGateImageResolver_UsesDefaultPrefixWhenEnvUnset(t *testing.T) {
	t.Setenv(containerRegistryEnvKey, "")

	resolver, err := NewBuildGateImageResolver("", []contracts.BuildGateImageRule{
		{
			Stack: contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"},
			Image: "$PLOY_CONTAINER_REGISTRY/maven:3-eclipse-temurin-17",
		},
	}, false)
	if err != nil {
		t.Fatalf("NewBuildGateImageResolver() error: %v", err)
	}

	got, err := resolver.Resolve(contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"})
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}
	want := defaultRegistryImagePrefix + "/maven:3-eclipse-temurin-17"
	if got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}
}

// TestBuildGateImageResolver_DuplicateInLevel tests duplicate rejection within same source.
func TestBuildGateImageResolver_DuplicateInLevel(t *testing.T) {
	// Duplicates within the same level should be rejected during validation.
	duplicateRules := []contracts.BuildGateImageRule{
		{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "image1:17"},
		{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "image2:17"},
	}

	_, err := NewBuildGateImageResolver("", duplicateRules, false)
	if err == nil {
		t.Fatal("expected duplicate validation error")
	}
	if !strings.Contains(err.Error(), "duplicate selector") {
		t.Errorf("error = %q, want to contain 'duplicate selector'", err.Error())
	}
}

// TestBuildGateImageResolver_LoadValidFile tests loading a valid YAML file.
func TestBuildGateImageResolver_LoadValidFile(t *testing.T) {
	testFile := filepath.Join("testdata", "stacks-catalog", "valid.yaml")
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Skip("test data file not found")
	}

	resolver, err := NewBuildGateImageResolver(testFile, nil, true)
	if err != nil {
		t.Fatalf("NewBuildGateImageResolver failed: %v", err)
	}

	// Test resolution.
	tests := []struct {
		name string
		exp  contracts.StackExpectation
		want string
	}{
		{
			name: "java 17 maven",
			exp:  contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"},
			want: "maven:3-eclipse-temurin-17",
		},
		{
			name: "java 17 gradle",
			exp:  contracts.StackExpectation{Language: "java", Release: "17", Tool: "gradle"},
			want: "gradle:8.8-jdk17",
		},
		{
			name: "java 17 fallback",
			exp:  contracts.StackExpectation{Language: "java", Release: "17"},
			want: "eclipse-temurin:17-jdk",
		},
		{
			name: "java 11 maven",
			exp:  contracts.StackExpectation{Language: "java", Release: "11", Tool: "maven"},
			want: "maven:3-eclipse-temurin-11",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolver.Resolve(tt.exp)
			if err != nil {
				t.Fatalf("Resolve failed: %v", err)
			}
			if got != tt.want {
				t.Errorf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildGateImageResolver_LoadFile_ReleaseCoercion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "stacks.yaml")
	writeDummyProfile(t, filepath.Join(dir, "profiles", "java-maven.yaml"))
	writeDummyProfile(t, filepath.Join(dir, "profiles", "java-gradle.yaml"))
	writeDummyProfile(t, filepath.Join(dir, "profiles", "python.yaml"))
	const body = `stacks:
  - lang: java
    tool: maven
    release: 17
    image: maven:17
    profile: profiles/java-maven.yaml
  - lang: java
    tool: gradle
    release: 17.0
    image: gradle:17
    profile: profiles/java-gradle.yaml
  - lang: python
    release: 3.11
    image: python:3.11
    profile: profiles/python.yaml
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write mapping file: %v", err)
	}

	resolver, err := NewBuildGateImageResolver(path, nil, true)
	if err != nil {
		t.Fatalf("NewBuildGateImageResolver() error: %v", err)
	}

	tests := []struct {
		name string
		exp  contracts.StackExpectation
		want string
	}{
		{
			name: "integer release",
			exp:  contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"},
			want: "maven:17",
		},
		{
			name: "whole float coerces to integer string",
			exp:  contracts.StackExpectation{Language: "java", Tool: "gradle", Release: "17"},
			want: "gradle:17",
		},
		{
			name: "decimal float preserved",
			exp:  contracts.StackExpectation{Language: "python", Release: "3.11"},
			want: "python:3.11",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolver.Resolve(tt.exp)
			if err != nil {
				t.Fatalf("Resolve() error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBuildGateImageResolver_LoadInvalidFile tests error handling for invalid files.
func TestBuildGateImageResolver_LoadInvalidFile(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		wantErr string
	}{
		{
			name:    "missing language",
			file:    filepath.Join("testdata", "stacks-catalog", "invalid-missing-language.yaml"),
			wantErr: "stacks[0].lang: required",
		},
		{
			name:    "duplicate selector",
			file:    filepath.Join("testdata", "stacks-catalog", "invalid-duplicate.yaml"),
			wantErr: "duplicate selector",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := os.Stat(tt.file); os.IsNotExist(err) {
				t.Skip("test data file not found")
			}

			_, err := NewBuildGateImageResolver(tt.file, nil, true)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestBuildGateImageResolver_EmptyRules tests error when no rules available.
func TestBuildGateImageResolver_EmptyRules(t *testing.T) {
	resolver := &BuildGateImageResolver{rules: nil}
	_, err := resolver.Resolve(contracts.StackExpectation{Language: "java", Release: "17"})
	if err == nil {
		t.Fatal("expected error for empty rules")
	}
	if !strings.Contains(err.Error(), "no image mapping rules available") {
		t.Errorf("error = %q, want to contain 'no image mapping rules available'", err.Error())
	}
}

// TestBuildGateImageResolver_SpecificityWins tests that more specific rules win over less specific.
func TestBuildGateImageResolver_SpecificityWins(t *testing.T) {
	rules := []contracts.BuildGateImageRule{
		// Tool-agnostic (specificity 2).
		{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "agnostic:17"},
		// Tool-specific (specificity 3) - should win.
		{Stack: contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"}, Image: "maven:17"},
	}

	resolver := &BuildGateImageResolver{rules: rules}

	// Request with maven tool should get maven-specific image.
	got, err := resolver.Resolve(contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "maven:17" {
		t.Errorf("Resolve() = %q, want %q (specificity should win)", got, "maven:17")
	}

	// Request with unknown tool should fall back to agnostic.
	got, err = resolver.Resolve(contracts.StackExpectation{Language: "java", Release: "17", Tool: "ant"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "agnostic:17" {
		t.Errorf("Resolve() = %q, want %q (should fallback to agnostic)", got, "agnostic:17")
	}
}

// TestBuildGateImageResolver_FullPrecedenceWithFile tests all three precedence levels
// with an actual file for the default rules.
func TestBuildGateImageResolver_FullPrecedenceWithFile(t *testing.T) {
	// Create temp file for default rules.
	tmpDir := t.TempDir()
	defaultFile := filepath.Join(tmpDir, "default.yaml")
	writeDummyProfile(t, filepath.Join(tmpDir, "profiles", "java17.yaml"))
	writeDummyProfile(t, filepath.Join(tmpDir, "profiles", "java11.yaml"))
	if err := os.WriteFile(defaultFile, []byte(`
stacks:
  - image: default:17
    lang: java
    release: "17"
    profile: profiles/java17.yaml
  - image: default:11
    lang: java
    release: "11"
    profile: profiles/java11.yaml
`), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	migRules := []contracts.BuildGateImageRule{
		{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "mig:17"},
	}

	t.Run("mig overrides default", func(t *testing.T) {
		resolver, err := NewBuildGateImageResolver(defaultFile, migRules, true)
		if err != nil {
			t.Fatalf("NewBuildGateImageResolver failed: %v", err)
		}

		got, err := resolver.Resolve(contracts.StackExpectation{Language: "java", Release: "17"})
		if err != nil {
			t.Fatalf("Resolve failed: %v", err)
		}
		if got != "mig:17" {
			t.Errorf("Resolve() = %q, want %q (mig should override default)", got, "mig:17")
		}
	})

	t.Run("default used when no overrides", func(t *testing.T) {
		resolver, err := NewBuildGateImageResolver(defaultFile, nil, true)
		if err != nil {
			t.Fatalf("NewBuildGateImageResolver failed: %v", err)
		}

		got, err := resolver.Resolve(contracts.StackExpectation{Language: "java", Release: "17"})
		if err != nil {
			t.Fatalf("Resolve failed: %v", err)
		}
		if got != "default:17" {
			t.Errorf("Resolve() = %q, want %q (should use default)", got, "default:17")
		}
	})

	t.Run("default used for non-overridden stack", func(t *testing.T) {
		// mig only overrides java:17, not java:11.
		resolver, err := NewBuildGateImageResolver(defaultFile, migRules, true)
		if err != nil {
			t.Fatalf("NewBuildGateImageResolver failed: %v", err)
		}

		got, err := resolver.Resolve(contracts.StackExpectation{Language: "java", Release: "11"})
		if err != nil {
			t.Fatalf("Resolve failed: %v", err)
		}
		if got != "default:11" {
			t.Errorf("Resolve() = %q, want %q (should fall back to default for java:11)", got, "default:11")
		}
	})
}

func TestBuildGateImageResolver_DefaultCatalogAssetsAreValid(t *testing.T) {
	catalogPath := buildGateDefaultStacksCatalogPath()
	resolver, err := NewBuildGateImageResolver(catalogPath, nil, true)
	if err != nil {
		t.Fatalf("NewBuildGateImageResolver() error: %v", err)
	}
	if resolver == nil {
		t.Fatal("resolver is nil")
	}
	if len(resolver.rules) == 0 {
		t.Fatal("resolver has no rules from default stacks catalog")
	}
}
