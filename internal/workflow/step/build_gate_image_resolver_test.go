package step

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

// buildResolverFromRules constructs a resolver directly without validation.
// Used by table-driven tests where the rule list is the unit under test.
func buildResolverFromRules(rules ...contracts.BuildGateImageRule) func(*testing.T) (*BuildGateImageResolver, error) {
	return func(*testing.T) (*BuildGateImageResolver, error) {
		return &BuildGateImageResolver{rules: rules}, nil
	}
}

// TestBuildGateImageResolver_Resolve exercises every positive resolution path:
// direct rule lists, precedence/specificity ordering, file loading, env/stack
// placeholder expansion, and release coercion. Each row constructs a resolver
// and asserts that Resolve(exp) returns wantImage.
func TestBuildGateImageResolver_Resolve(t *testing.T) {
	defaultRules := []contracts.BuildGateImageRule{
		{Stack: contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"}, Image: "maven:jdk17"},
		{Stack: contracts.StackExpectation{Language: "java", Release: "17", Tool: "gradle"}, Image: "gradle:8.8-jdk17"},
		{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "eclipse-temurin:17-jdk"},
		{Stack: contracts.StackExpectation{Language: "java", Release: "11"}, Image: "eclipse-temurin:11-jdk"},
	}

	// Shared file-backed fixtures.
	releaseCoercionDir := t.TempDir()
	releaseCoercionPath := filepath.Join(releaseCoercionDir, "gates.yaml")
	if err := os.WriteFile(releaseCoercionPath, []byte(`gates:
  - lang: java
    tool: maven
    release: 17
    image: maven:17
  - lang: java
    tool: gradle
    release: 17.0
    image: gradle:17
  - lang: python
    release: 3.11
    image: python:3.11
`), 0o644); err != nil {
		t.Fatalf("write release-coercion mapping file: %v", err)
	}

	fullPrecedenceDir := t.TempDir()
	fullPrecedencePath := filepath.Join(fullPrecedenceDir, "default.yaml")
	if err := os.WriteFile(fullPrecedencePath, []byte(`
gates:
  - image: default:17
    lang: java
    release: "17"
  - image: default:11
    lang: java
    release: "11"
`), 0o644); err != nil {
		t.Fatalf("write full-precedence mapping file: %v", err)
	}
	migOverride := []contracts.BuildGateImageRule{
		{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "mig:17"},
	}

	validFile := filepath.Join("testdata", "stacks-catalog", "valid.yaml")
	loadValidFile := func(t *testing.T) (*BuildGateImageResolver, error) {
		if _, err := os.Stat(validFile); os.IsNotExist(err) {
			t.Skip("test data file not found")
		}
		return NewBuildGateImageResolver(validFile, nil, true)
	}

	tests := []struct {
		name      string
		setup     func(t *testing.T) (*BuildGateImageResolver, error)
		exp       contracts.StackExpectation
		wantImage string
	}{
		// Direct rules: exact match + tool-agnostic fallbacks.
		{"exact match maven", buildResolverFromRules(defaultRules...), contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"}, "maven:jdk17"},
		{"exact match gradle", buildResolverFromRules(defaultRules...), contracts.StackExpectation{Language: "java", Release: "17", Tool: "gradle"}, "gradle:8.8-jdk17"},
		{"tool-agnostic fallback unknown tool", buildResolverFromRules(defaultRules...), contracts.StackExpectation{Language: "java", Release: "17", Tool: "ant"}, "eclipse-temurin:17-jdk"},
		{"tool-agnostic fallback no tool", buildResolverFromRules(defaultRules...), contracts.StackExpectation{Language: "java", Release: "17"}, "eclipse-temurin:17-jdk"},
		{"different release", buildResolverFromRules(defaultRules...), contracts.StackExpectation{Language: "java", Release: "11"}, "eclipse-temurin:11-jdk"},

		// Precedence: mig override beats default; default used otherwise; last wins at same specificity.
		{"mig overrides default", buildResolverFromRules(
			contracts.BuildGateImageRule{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "default:17"},
			contracts.BuildGateImageRule{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "mig:17"},
		), contracts.StackExpectation{Language: "java", Release: "17"}, "mig:17"},
		{"default used when no mig override", buildResolverFromRules(
			contracts.BuildGateImageRule{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "default:17"},
		), contracts.StackExpectation{Language: "java", Release: "17"}, "default:17"},
		{"last rule wins at same specificity", buildResolverFromRules(
			contracts.BuildGateImageRule{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "image1:17"},
			contracts.BuildGateImageRule{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "image2:17"},
		), contracts.StackExpectation{Language: "java", Release: "17"}, "image2:17"},

		// Specificity: more-specific rule wins over tool-agnostic.
		{"specificity wins over agnostic", buildResolverFromRules(
			contracts.BuildGateImageRule{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "agnostic:17"},
			contracts.BuildGateImageRule{Stack: contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"}, Image: "maven:17"},
		), contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"}, "maven:17"},
		{"unknown tool falls back to agnostic", buildResolverFromRules(
			contracts.BuildGateImageRule{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "agnostic:17"},
			contracts.BuildGateImageRule{Stack: contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"}, Image: "maven:17"},
		), contracts.StackExpectation{Language: "java", Release: "17", Tool: "ant"}, "agnostic:17"},

		// Env / placeholder expansion via NewBuildGateImageResolver.
		{"expands $PLOY_CONTAINER_REGISTRY", func(t *testing.T) (*BuildGateImageResolver, error) {
			t.Setenv("PLOY_CONTAINER_REGISTRY", "192.0.2.25:5001/ploy")
			return NewBuildGateImageResolver("", []contracts.BuildGateImageRule{{
				Stack: contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"},
				Image: "$PLOY_CONTAINER_REGISTRY/maven:jdk17",
			}}, false)
		}, contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"}, "192.0.2.25:5001/ploy/maven:jdk17"},
		{"expands ${stack.*} placeholders", func(*testing.T) (*BuildGateImageResolver, error) {
			return NewBuildGateImageResolver("", []contracts.BuildGateImageRule{{
				Stack: contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"},
				Image: "ghcr.io/acme/mig-${stack.language}-${stack.release}-${stack.tool}:latest",
			}}, false)
		}, contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"}, "ghcr.io/acme/mig-java-17-maven:latest"},

		// File loading: shipped testdata.
		{"file java 17 maven", loadValidFile, contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"}, "maven:jdk17"},
		{"file java 17 gradle", loadValidFile, contracts.StackExpectation{Language: "java", Release: "17", Tool: "gradle"}, "gradle:8.8-jdk17"},
		{"file java 17 fallback", loadValidFile, contracts.StackExpectation{Language: "java", Release: "17"}, "eclipse-temurin:17-jdk"},
		{"file java 11 maven", loadValidFile, contracts.StackExpectation{Language: "java", Release: "11", Tool: "maven"}, "maven:jdk11"},

		// Release coercion (integer / whole-float / decimal-float in YAML).
		{"file release int coerced", func(*testing.T) (*BuildGateImageResolver, error) {
			return NewBuildGateImageResolver(releaseCoercionPath, nil, true)
		}, contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"}, "maven:17"},
		{"file release whole-float coerced", func(*testing.T) (*BuildGateImageResolver, error) {
			return NewBuildGateImageResolver(releaseCoercionPath, nil, true)
		}, contracts.StackExpectation{Language: "java", Tool: "gradle", Release: "17"}, "gradle:17"},
		{"file release decimal preserved", func(*testing.T) (*BuildGateImageResolver, error) {
			return NewBuildGateImageResolver(releaseCoercionPath, nil, true)
		}, contracts.StackExpectation{Language: "python", Release: "3.11"}, "python:3.11"},

		// Full precedence: file + mig overrides combined.
		{"file+mig: mig overrides default", func(*testing.T) (*BuildGateImageResolver, error) {
			return NewBuildGateImageResolver(fullPrecedencePath, migOverride, true)
		}, contracts.StackExpectation{Language: "java", Release: "17"}, "mig:17"},
		{"file+mig: default used when not overridden", func(*testing.T) (*BuildGateImageResolver, error) {
			return NewBuildGateImageResolver(fullPrecedencePath, migOverride, true)
		}, contracts.StackExpectation{Language: "java", Release: "11"}, "default:11"},
		{"file: default used when no mig provided", func(*testing.T) (*BuildGateImageResolver, error) {
			return NewBuildGateImageResolver(fullPrecedencePath, nil, true)
		}, contracts.StackExpectation{Language: "java", Release: "17"}, "default:17"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver, err := tt.setup(t)
			if err != nil {
				t.Fatalf("setup error: %v", err)
			}
			got, err := resolver.Resolve(tt.exp)
			if err != nil {
				t.Fatalf("Resolve() error: %v", err)
			}
			if got != tt.wantImage {
				t.Errorf("Resolve() = %q, want %q", got, tt.wantImage)
			}
		})
	}
}

// TestBuildGateImageResolver_Errors exercises every error path: empty rules,
// no-match expectations, missing required file, unresolved env, duplicate
// selectors, and invalid YAML files. Each row errors either during setup
// (wantSetupErr non-empty) or during Resolve (wantResolveErr non-empty).
func TestBuildGateImageResolver_Errors(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T) (*BuildGateImageResolver, error)
		exp            contracts.StackExpectation
		wantSetupErr   string
		wantResolveErr string
	}{
		{
			name:           "no match wrong language",
			setup:          buildResolverFromRules(contracts.BuildGateImageRule{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "x:17"}),
			exp:            contracts.StackExpectation{Language: "go", Release: "1.21"},
			wantResolveErr: "no image rule matches stack go:1.21:",
		},
		{
			name:           "no match wrong release",
			setup:          buildResolverFromRules(contracts.BuildGateImageRule{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "x:17"}),
			exp:            contracts.StackExpectation{Language: "java", Release: "21"},
			wantResolveErr: "no image rule matches stack java:21:",
		},
		{
			name:           "empty rules",
			setup:          buildResolverFromRules(),
			exp:            contracts.StackExpectation{Language: "java", Release: "17"},
			wantResolveErr: "no image mapping rules available",
		},
		{
			name: "missing required file",
			setup: func(*testing.T) (*BuildGateImageResolver, error) {
				return NewBuildGateImageResolver("/nonexistent/path/gates.yaml", nil, true)
			},
			wantSetupErr: "required but not found",
		},
		{
			name: "duplicate selector within same level",
			setup: func(*testing.T) (*BuildGateImageResolver, error) {
				return NewBuildGateImageResolver("", []contracts.BuildGateImageRule{
					{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "image1:17"},
					{Stack: contracts.StackExpectation{Language: "java", Release: "17"}, Image: "image2:17"},
				}, false)
			},
			wantSetupErr: "duplicate selector",
		},
		{
			name: "unresolved env var fails when unset",
			setup: func(*testing.T) (*BuildGateImageResolver, error) {
				return NewBuildGateImageResolver("", []contracts.BuildGateImageRule{{
					Stack: contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"},
					Image: "$PLOY_TEST_UNSET_GATE_REGISTRY/maven:jdk17",
				}}, false)
			},
			wantSetupErr: "unresolved environment variables: PLOY_TEST_UNSET_GATE_REGISTRY",
		},
		{
			name: "file missing language",
			setup: func(t *testing.T) (*BuildGateImageResolver, error) {
				path := filepath.Join("testdata", "stacks-catalog", "invalid-missing-language.yaml")
				if _, err := os.Stat(path); os.IsNotExist(err) {
					t.Skip("test data file not found")
				}
				return NewBuildGateImageResolver(path, nil, true)
			},
			wantSetupErr: "gates[0].lang: required",
		},
		{
			name: "file duplicate selector",
			setup: func(t *testing.T) (*BuildGateImageResolver, error) {
				path := filepath.Join("testdata", "stacks-catalog", "invalid-duplicate.yaml")
				if _, err := os.Stat(path); os.IsNotExist(err) {
					t.Skip("test data file not found")
				}
				return NewBuildGateImageResolver(path, nil, true)
			},
			wantSetupErr: "duplicate selector",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver, err := tt.setup(t)
			if tt.wantSetupErr != "" {
				requireErrContains(t, err, tt.wantSetupErr)
				return
			}
			if err != nil {
				t.Fatalf("setup error: %v", err)
			}
			_, err = resolver.Resolve(tt.exp)
			requireErrContains(t, err, tt.wantResolveErr)
		})
	}
}

// TestBuildGateImageResolver_MissingFileOptional verifies that a missing
// default-file path is tolerated when requireDefaultFile=false.
func TestBuildGateImageResolver_MissingFileOptional(t *testing.T) {
	resolver, err := NewBuildGateImageResolver("/nonexistent/path/gates.yaml", nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolver == nil {
		t.Fatal("resolver is nil")
	}
}

func TestBuildGateImageResolver_DefaultCatalogAssetsAreValid(t *testing.T) {
	catalogPath := buildGateDefaultGatesCatalogPath()
	resolver, err := NewBuildGateImageResolver(catalogPath, nil, true)
	if err != nil {
		t.Fatalf("NewBuildGateImageResolver() error: %v", err)
	}
	if resolver == nil {
		t.Fatal("resolver is nil")
	}
	if len(resolver.rules) == 0 {
		t.Fatal("resolver has no rules from default gates catalog")
	}
}
