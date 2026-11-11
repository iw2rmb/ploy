package step

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
)

func imageRule(language, release, tool, image string) contracts.BuildGateImageRule {
	return contracts.BuildGateImageRule{
		Stack: contracts.StackExpectation{Language: language, Release: release, Tool: tool},
		Image: image,
	}
}

func writeCatalog(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gates.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write gates catalog: %v", err)
	}
	return path
}

// TestSelectImage covers rule selection over an already merged rule list:
// specificity ordering, tool-agnostic fallback, and last-rule-wins precedence.
func TestSelectImage(t *testing.T) {
	defaultRules := []contracts.BuildGateImageRule{
		imageRule("java", "17", "maven", "maven:jdk17"),
		imageRule("java", "17", "gradle", "gradle:8.8-jdk17"),
		imageRule("java", "17", "", "eclipse-temurin:17-jdk"),
		imageRule("java", "11", "", "eclipse-temurin:11-jdk"),
	}

	tests := []struct {
		name      string
		rules     []contracts.BuildGateImageRule
		exp       contracts.StackExpectation
		wantImage string
	}{
		{"exact match maven", defaultRules, contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"}, "maven:jdk17"},
		{"exact match gradle", defaultRules, contracts.StackExpectation{Language: "java", Release: "17", Tool: "gradle"}, "gradle:8.8-jdk17"},
		{"tool-agnostic fallback unknown tool", defaultRules, contracts.StackExpectation{Language: "java", Release: "17", Tool: "ant"}, "eclipse-temurin:17-jdk"},
		{"tool-agnostic fallback no tool", defaultRules, contracts.StackExpectation{Language: "java", Release: "17"}, "eclipse-temurin:17-jdk"},
		{"different release", defaultRules, contracts.StackExpectation{Language: "java", Release: "11"}, "eclipse-temurin:11-jdk"},
		{"later rule overrides earlier", []contracts.BuildGateImageRule{
			imageRule("java", "17", "", "default:17"),
			imageRule("java", "17", "", "mig:17"),
		}, contracts.StackExpectation{Language: "java", Release: "17"}, "mig:17"},
		{"specificity wins over later agnostic rule", []contracts.BuildGateImageRule{
			imageRule("java", "17", "maven", "maven:17"),
			imageRule("java", "17", "", "agnostic:17"),
		}, contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"}, "maven:17"},
		{"specificity wins over earlier agnostic rule", []contracts.BuildGateImageRule{
			imageRule("java", "17", "", "agnostic:17"),
			imageRule("java", "17", "maven", "maven:17"),
		}, contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"}, "maven:17"},
		{"unknown tool falls back to agnostic", []contracts.BuildGateImageRule{
			imageRule("java", "17", "", "agnostic:17"),
			imageRule("java", "17", "maven", "maven:17"),
		}, contracts.StackExpectation{Language: "java", Release: "17", Tool: "ant"}, "agnostic:17"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectImage(tt.rules, tt.exp)
			if err != nil {
				t.Fatalf("selectImage() error: %v", err)
			}
			if got != tt.wantImage {
				t.Errorf("selectImage() = %q, want %q", got, tt.wantImage)
			}
		})
	}
}

// TestResolveImageForExpectation covers loading from the catalog file and
// overrides, release coercion, and template expansion at resolve time.
func TestResolveImageForExpectation(t *testing.T) {
	releaseCoercionPath := writeCatalog(t, `gates:
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
`)
	precedencePath := writeCatalog(t, `
gates:
  - image: default:17
    lang: java
    release: "17"
  - image: default:11
    lang: java
    release: "11"
`)
	migOverride := []contracts.BuildGateImageRule{imageRule("java", "17", "", "mig:17")}
	validFile := filepath.Join("testdata", "stacks-catalog", "valid.yaml")

	tests := []struct {
		name        string
		env         map[string]string
		catalogPath string
		overrides   []contracts.BuildGateImageRule
		exp         contracts.StackExpectation
		wantImage   string
	}{
		{
			name:      "expands $PLOY_CONTAINER_REGISTRY",
			env:       map[string]string{"PLOY_CONTAINER_REGISTRY": "192.0.2.25:5001/ploy"},
			overrides: []contracts.BuildGateImageRule{imageRule("java", "17", "maven", "$PLOY_CONTAINER_REGISTRY/maven:jdk17")},
			exp:       contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"},
			wantImage: "192.0.2.25:5001/ploy/maven:jdk17",
		},
		{
			name:      "expands ${stack.*} placeholders",
			overrides: []contracts.BuildGateImageRule{imageRule("java", "17", "maven", "ghcr.io/acme/mig-${stack.language}-${stack.release}-${stack.tool}:latest")},
			exp:       contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"},
			wantImage: "ghcr.io/acme/mig-java-17-maven:latest",
		},
		{
			name:      "language-only rule expands placeholders from runtime stack",
			overrides: []contracts.BuildGateImageRule{imageRule("java", "", "", "temurin:${stack.release}-${stack.tool}")},
			exp:       contracts.StackExpectation{Language: "java", Release: "21", Tool: "gradle"},
			wantImage: "temurin:21-gradle",
		},
		{
			name: "unselected rule with unresolvable template does not fail",
			overrides: []contracts.BuildGateImageRule{
				imageRule("java", "", "", "$PLOY_TEST_UNSET_GATE_REGISTRY/java"),
				imageRule("java", "17", "", "temurin:17"),
			},
			exp:       contracts.StackExpectation{Language: "java", Release: "17"},
			wantImage: "temurin:17",
		},
		{"file java 17 maven", nil, validFile, nil, contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"}, "maven:jdk17"},
		{"file java 17 gradle", nil, validFile, nil, contracts.StackExpectation{Language: "java", Release: "17", Tool: "gradle"}, "gradle:8.8-jdk17"},
		{"file java 17 fallback", nil, validFile, nil, contracts.StackExpectation{Language: "java", Release: "17"}, "eclipse-temurin:17-jdk"},
		{"file java 11 maven", nil, validFile, nil, contracts.StackExpectation{Language: "java", Release: "11", Tool: "maven"}, "maven:jdk11"},
		{"file release int coerced", nil, releaseCoercionPath, nil, contracts.StackExpectation{Language: "java", Tool: "maven", Release: "17"}, "maven:17"},
		{"file release whole-float coerced", nil, releaseCoercionPath, nil, contracts.StackExpectation{Language: "java", Tool: "gradle", Release: "17"}, "gradle:17"},
		{"file release decimal preserved", nil, releaseCoercionPath, nil, contracts.StackExpectation{Language: "python", Release: "3.11"}, "python:3.11"},
		{"file+mig: mig overrides default", nil, precedencePath, migOverride, contracts.StackExpectation{Language: "java", Release: "17"}, "mig:17"},
		{"file+mig: default used when not overridden", nil, precedencePath, migOverride, contracts.StackExpectation{Language: "java", Release: "11"}, "default:11"},
		{"file: default used when no mig provided", nil, precedencePath, nil, contracts.StackExpectation{Language: "java", Release: "17"}, "default:17"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got, err := resolveImageForExpectation(tt.catalogPath, tt.overrides, tt.exp)
			if err != nil {
				t.Fatalf("resolveImageForExpectation() error: %v", err)
			}
			if got != tt.wantImage {
				t.Errorf("resolveImageForExpectation() = %q, want %q", got, tt.wantImage)
			}
		})
	}
}

// TestResolveImageForExpectation_Errors covers every error path. Rule sources
// that fail to load or validate wrap errImageMapping; selection and template
// failures wrap errImageRuleMatch.
func TestResolveImageForExpectation_Errors(t *testing.T) {
	tests := []struct {
		name        string
		catalogPath string
		overrides   []contracts.BuildGateImageRule
		exp         contracts.StackExpectation
		wantIs      error
		wantErr     string
	}{
		{
			name:      "no match wrong language",
			overrides: []contracts.BuildGateImageRule{imageRule("java", "17", "", "x:17")},
			exp:       contracts.StackExpectation{Language: "go", Release: "1.21"},
			wantIs:    errImageRuleMatch,
			wantErr:   "no image rule matches stack go:1.21:",
		},
		{
			name:      "no match wrong release",
			overrides: []contracts.BuildGateImageRule{imageRule("java", "17", "", "x:17")},
			exp:       contracts.StackExpectation{Language: "java", Release: "21"},
			wantIs:    errImageRuleMatch,
			wantErr:   "no image rule matches stack java:21:",
		},
		{
			name:    "empty rules",
			exp:     contracts.StackExpectation{Language: "java", Release: "17"},
			wantIs:  errImageRuleMatch,
			wantErr: "no image mapping rules available",
		},
		{
			name:        "missing catalog file",
			catalogPath: "/nonexistent/path/gates.yaml",
			wantIs:      errImageMapping,
			wantErr:     "required but not found",
		},
		{
			name: "duplicate selector within same level",
			overrides: []contracts.BuildGateImageRule{
				imageRule("java", "17", "", "image1:17"),
				imageRule("java", "17", "", "image2:17"),
			},
			wantIs:  errImageMapping,
			wantErr: "duplicate selector",
		},
		{
			name:      "selected rule with unset env var",
			overrides: []contracts.BuildGateImageRule{imageRule("java", "17", "maven", "$PLOY_TEST_UNSET_GATE_REGISTRY/maven:jdk17")},
			exp:       contracts.StackExpectation{Language: "java", Release: "17", Tool: "maven"},
			wantIs:    errImageRuleMatch,
			wantErr:   "unresolved environment variables: PLOY_TEST_UNSET_GATE_REGISTRY",
		},
		{
			name:        "file missing language",
			catalogPath: filepath.Join("testdata", "stacks-catalog", "invalid-missing-language.yaml"),
			wantIs:      errImageMapping,
			wantErr:     "gates[0].lang: required",
		},
		{
			name:        "file duplicate selector",
			catalogPath: filepath.Join("testdata", "stacks-catalog", "invalid-duplicate.yaml"),
			wantIs:      errImageMapping,
			wantErr:     "duplicate selector",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveImageForExpectation(tt.catalogPath, tt.overrides, tt.exp)
			requireErrContains(t, err, tt.wantErr)
			if !errors.Is(err, tt.wantIs) {
				t.Fatalf("error = %v, want errors.Is %v", err, tt.wantIs)
			}
		})
	}
}

func TestDefaultCatalogAssetsAreValid(t *testing.T) {
	rules, err := loadImageRules(defaultCatalogFilePath(), nil)
	if err != nil {
		t.Fatalf("loadImageRules() error: %v", err)
	}
	if len(rules) == 0 {
		t.Fatal("default gates catalog has no rules")
	}
}
