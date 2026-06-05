package stackdetect

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeJavaVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1.8", "8"},
		{"1.7", "7"},
		{"1.6", "6"},
		{"1.5", "5"},
		{"8", "8"},
		{"11", "11"},
		{"17", "17"},
		{"21", "21"},
		{"  17  ", "17"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeJavaVersion(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeJavaVersion(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSourceCompatibilityRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "quoted string",
			input:    `sourceCompatibility = "17"`,
			expected: "17",
		},
		{
			name:     "unquoted number",
			input:    `sourceCompatibility = 11`,
			expected: "11",
		},
		{
			name:     "java version constant (qualified)",
			input:    `sourceCompatibility = JavaVersion.VERSION_17`,
			expected: "17",
		},
		{
			name:     "java version constant (unqualified)",
			input:    `sourceCompatibility = VERSION_21`,
			expected: "21",
		},
		{
			name:     "legacy java version constant",
			input:    `sourceCompatibility = JavaVersion.VERSION_1_8`,
			expected: "8",
		},
		{
			name:     "with spaces",
			input:    `sourceCompatibility   =   21`,
			expected: "21",
		},
		{
			name:     "no match",
			input:    `targetCompatibility = 17`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCompatibilityVersion(sourceCompatibilityRegex, tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestTargetCompatibilityRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "quoted string",
			input:    `targetCompatibility = "17"`,
			expected: "17",
		},
		{
			name:     "unquoted number",
			input:    `targetCompatibility = 11`,
			expected: "11",
		},
		{
			name:     "java version constant",
			input:    `targetCompatibility = JavaVersion.VERSION_17`,
			expected: "17",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCompatibilityVersion(targetCompatibilityRegex, tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDynamicPatterns(t *testing.T) {
	dynamicInputs := []string{
		`val javaVersion = findProperty("javaVersion")`,
		`def version = getProperty("version")`,
		`val jdk = System.getenv("JAVA_HOME")`,
		`val prop = project.properties["javaVersion"]`,
		`val ver = extra["javaVersion"]`,
		`def ver = ext["javaVersion"]`,
		`val javaVer = if (condition) JavaVersion.VERSION_17 else JavaVersion.VERSION_11`,
	}

	for _, input := range dynamicInputs {
		name := input
		if len(name) > 40 {
			name = name[:40]
		}
		t.Run(name, func(t *testing.T) {
			matched := false
			for _, pattern := range dynamicPatterns {
				if pattern.MatchString(input) {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("expected dynamic pattern match for: %s", input)
			}
		})
	}

	staticInputs := []string{
		`sourceCompatibility = 17`,
		`kotlinOptions.jvmTarget = "17"`,
	}

	for _, input := range staticInputs {
		t.Run("static:"+input, func(t *testing.T) {
			matched := false
			for _, pattern := range dynamicPatterns {
				if pattern.MatchString(input) {
					matched = true
					break
				}
			}
			if matched {
				t.Errorf("unexpected dynamic pattern match for static input: %s", input)
			}
		})
	}
}

func TestKotlinOptionsJvmTargetDirectRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "quoted number",
			input:    `kotlinOptions.jvmTarget = "17"`,
			expected: "17",
		},
		{
			name:     "java version constant",
			input:    `kotlinOptions.jvmTarget = JavaVersion.VERSION_21`,
			expected: "21",
		},
		{
			name:     "legacy java version constant",
			input:    `kotlinOptions.jvmTarget = JavaVersion.VERSION_1_8`,
			expected: "8",
		},
		{
			name:     "no match",
			input:    `kotlinOptions { jvmTarget = "17" }`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCompatibilityVersion(kotlinOptionsJvmTargetDirectRegex, tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestKotlinOptionsJvmTargetBlockRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single line",
			input:    `kotlinOptions { jvmTarget = "17" }`,
			expected: "17",
		},
		{
			name: "multi line",
			input: `
kotlinOptions {
    jvmTarget = JavaVersion.VERSION_21
}
`,
			expected: "21",
		},
		{
			name:     "no match",
			input:    `kotlinOptions.jvmTarget = "17"`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCompatibilityVersion(kotlinOptionsJvmTargetBlockRegex, tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestJavaVersionAssignmentRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "qualified constant",
			input:    `javaVersion = JavaVersion.VERSION_21`,
			expected: "21",
		},
		{
			name:     "unqualified constant",
			input:    `javaVersion = VERSION_17`,
			expected: "17",
		},
		{
			name:     "legacy constant",
			input:    `javaVersion = JavaVersion.VERSION_1_8`,
			expected: "8",
		},
		{
			name:     "numeric assignment",
			input:    `javaVersion = 11`,
			expected: "11",
		},
		{
			name:     "java language version factory",
			input:    `javaVersion = JavaLanguageVersion.of("21")`,
			expected: "21",
		},
		{
			name:     "no match",
			input:    `sourceCompatibility = 17`,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCompatibilityVersion(javaVersionAssignmentRegex, tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDetectGradle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		fileName        string
		content         string
		extraFiles      map[string]string
		wantRelease     string
		wantEvidenceKey string
		wantError       bool
	}{
		{
			name:     "ext properties with explicit compatibility",
			fileName: "build.gradle",
			content: `import static org.gradle.api.JavaVersion.VERSION_11

plugins { id 'java' }

sourceCompatibility = VERSION_11
targetCompatibility = VERSION_11

ext['log4j2.version'] = '2.16.0'
`,
			wantRelease: "11",
		},
		{
			name:     "toolchain languageVersion assign",
			fileName: "build.gradle",
			content: `
plugins { id "java" }

java {
    toolchain {
        languageVersion = JavaLanguageVersion.of(17)
    }
}
`,
			wantRelease: "17",
		},
		{
			name:     "toolchain languageVersion assign JavaVersion constant",
			fileName: "build.gradle.kts",
			content: `
plugins { java }

java {
    toolchain {
        languageVersion = JavaVersion.VERSION_21
    }
}
`,
			wantRelease: "21",
		},
		{
			name:     "toolchain languageVersion assign unqualified constant",
			fileName: "build.gradle.kts",
			content: `
plugins { java }

java {
    toolchain {
        languageVersion = VERSION_17
    }
}
`,
			wantRelease: "17",
		},
		{
			name:     "toolchain languageVersion assign numeric",
			fileName: "build.gradle.kts",
			content: `
plugins { java }

java {
    toolchain {
        languageVersion = 11
    }
}
`,
			wantRelease: "11",
		},
		{
			name:     "toolchain languageVersion set KTS",
			fileName: "build.gradle.kts",
			content: `
plugins { java }

java {
    toolchain {
        languageVersion.set(JavaLanguageVersion.of("21"))
    }
}
`,
			wantRelease: "21",
		},
		{
			name:     "toolchain languageVersion set JavaVersion constant",
			fileName: "build.gradle.kts",
			content: `
plugins { java }

java {
    toolchain {
        languageVersion.set(JavaVersion.VERSION_17)
    }
}
`,
			wantRelease: "17",
		},
		{
			name:     "toolchain languageVersion set unqualified constant",
			fileName: "build.gradle.kts",
			content: `
plugins { java }

java {
    toolchain {
        languageVersion.set(VERSION_11)
    }
}
`,
			wantRelease: "11",
		},
		{
			name:     "toolchain languageVersion set numeric",
			fileName: "build.gradle.kts",
			content: `
plugins { java }

java {
    toolchain {
        languageVersion.set(21)
    }
}
`,
			wantRelease: "21",
		},
		{
			name:     "dependency manager javaVersion assignment qualified",
			fileName: "build.gradle.kts",
			content: `
dependencyManagerRootExtension {
    javaVersion = JavaVersion.VERSION_21
}
`,
			wantRelease: "21",
		},
		{
			name:     "dependency manager javaVersion assignment JavaLanguageVersion factory",
			fileName: "build.gradle.kts",
			content: `
dependencyManagerRootExtension {
    javaVersion = JavaLanguageVersion.of(21)
}
`,
			wantRelease: "21",
		},
		{
			name:     "dependency manager javaVersion assignment unqualified",
			fileName: "build.gradle.kts",
			content: `
dependencyManagerRootExtension {
    javaVersion = VERSION_17
}
`,
			wantRelease: "17",
		},
		{
			name:     "dependency manager javaVersion set qualified",
			fileName: "build.gradle.kts",
			content: `
dependencyManagerRootExtension {
    javaVersion.set(JavaVersion.VERSION_21)
}
`,
			wantRelease: "21",
		},
		{
			name:     "dependency manager javaVersion set unqualified",
			fileName: "build.gradle.kts",
			content: `
dependencyManagerRootExtension {
    javaVersion.set(VERSION_17)
}
`,
			wantRelease: "17",
		},
		{
			name:     "dependency manager javaVersion set numeric",
			fileName: "build.gradle.kts",
			content: `
dependencyManagerRootExtension {
    javaVersion.set(11)
}
`,
			wantRelease: "11",
		},
		{
			name:     "dependency manager javaVersion set JavaLanguageVersion factory",
			fileName: "build.gradle.kts",
			content: `
dependencyManagerRootExtension {
    javaVersion.set(JavaLanguageVersion.of("21"))
}
`,
			wantRelease: "21",
		},
		{
			name:     "version catalog jvm target",
			fileName: "build.gradle",
			content: `
plugins { id "java" }
`,
			extraFiles: map[string]string{
				"gradle/libs.versions.toml": `
[versions]
jvmTarget = "17"
`,
			},
			wantRelease:     "17",
			wantEvidenceKey: "versions.jvmTarget",
		},
		{
			name:     "malformed version catalog",
			fileName: "build.gradle",
			content: `
plugins { id "java" }
`,
			extraFiles: map[string]string{
				"gradle/libs.versions.toml": `
[versions
jvmTarget = "17"
`,
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			workspace := t.TempDir()
			gradlePath := filepath.Join(workspace, tt.fileName)
			if err := os.WriteFile(gradlePath, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write %s: %v", tt.fileName, err)
			}
			for rel, content := range tt.extraFiles {
				path := filepath.Join(workspace, rel)
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
				}
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatalf("write %s: %v", rel, err)
				}
			}

			obs, err := detectGradle(context.Background(), workspace, gradlePath)
			if tt.wantError {
				if err == nil {
					t.Fatal("detectGradle error = nil, want non-nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("detectGradle error: %v", err)
			}
			if obs == nil || obs.Release == nil {
				t.Fatal("nil observation or release")
			}
			if got := *obs.Release; got != tt.wantRelease {
				t.Errorf("release = %q, want %q", got, tt.wantRelease)
			}
			if obs.Tool != "gradle" {
				t.Errorf("tool = %q, want %q", obs.Tool, "gradle")
			}
			if obs.Language != "java" {
				t.Errorf("language = %q, want %q", obs.Language, "java")
			}
			if tt.wantEvidenceKey != "" {
				if len(obs.Evidence) != 1 {
					t.Fatalf("evidence len = %d, want 1", len(obs.Evidence))
				}
				evidence := obs.Evidence[0]
				if evidence.Path != "gradle/libs.versions.toml" {
					t.Errorf("evidence path = %q, want %q", evidence.Path, "gradle/libs.versions.toml")
				}
				if evidence.Key != tt.wantEvidenceKey {
					t.Errorf("evidence key = %q, want %q", evidence.Key, tt.wantEvidenceKey)
				}
				if evidence.Value != tt.wantRelease {
					t.Errorf("evidence value = %q, want %q", evidence.Value, tt.wantRelease)
				}
			}
		})
	}
}
