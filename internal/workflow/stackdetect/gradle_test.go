package stackdetect

import (
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

func TestJavaLanguageVersionRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "toolchain with of(17)",
			input:    `languageVersion.set(JavaLanguageVersion.of(17))`,
			expected: "17",
		},
		{
			name:     "toolchain with of(21)",
			input:    `languageVersion.set(JavaLanguageVersion.of(21))`,
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
			matches := javaLanguageVersionRegex.FindStringSubmatch(tt.input)
			var result string
			if matches != nil && len(matches) > 1 {
				result = matches[1]
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
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
		`val javaLangVer = if (condition) JavaLanguageVersion.of(17) else JavaLanguageVersion.of(11)`,
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
		`languageVersion.set(JavaLanguageVersion.of(17))`,
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
