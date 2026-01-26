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

func TestDetectGradle_AllowsExtPropertiesWhenCompatibilityExplicit(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	gradlePath := filepath.Join(workspace, "build.gradle")

	// This mirrors real-world Gradle builds that use ext[...] but still have an
	// explicit, static compatibility configuration.
	gradle := `import static org.gradle.api.JavaVersion.VERSION_11

plugins { id 'java' }

sourceCompatibility = VERSION_11
targetCompatibility = VERSION_11

ext['log4j2.version'] = '2.16.0'
`
	if err := os.WriteFile(gradlePath, []byte(gradle), 0o600); err != nil {
		t.Fatalf("write build.gradle: %v", err)
	}

	obs, err := detectGradle(context.Background(), workspace, gradlePath)
	if err != nil {
		t.Fatalf("detectGradle returned error: %v", err)
	}
	if obs == nil || obs.Release == nil {
		t.Fatalf("detectGradle returned nil observation or release")
	}
	if got, want := *obs.Release, "11"; got != want {
		t.Fatalf("release mismatch: got %q want %q", got, want)
	}
	if got, want := obs.Tool, "gradle"; got != want {
		t.Fatalf("tool mismatch: got %q want %q", got, want)
	}
	if got, want := obs.Language, "java"; got != want {
		t.Fatalf("language mismatch: got %q want %q", got, want)
	}
}
