package stackdetect

import (
	"errors"
	"strings"
	"testing"
)

func TestParseProperties(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:  "single property",
			input: `<java.version>17</java.version>`,
			expected: map[string]string{
				"java.version": "17",
			},
		},
		{
			name: "multiple properties",
			input: `
				<java.version>11</java.version>
				<maven.compiler.source>11</maven.compiler.source>
				<maven.compiler.target>11</maven.compiler.target>
			`,
			expected: map[string]string{
				"java.version":          "11",
				"maven.compiler.source": "11",
				"maven.compiler.target": "11",
			},
		},
		{
			name:     "empty properties",
			input:    "",
			expected: map[string]string{},
		},
		{
			name:  "property with whitespace",
			input: `<java.version>  17  </java.version>`,
			expected: map[string]string{
				"java.version": "17",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseProperties([]byte(tt.input))

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d properties, got %d", len(tt.expected), len(result))
			}

			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("expected %s=%q, got %q", k, v, result[k])
				}
			}
		})
	}
}

func TestResolveValue(t *testing.T) {
	props := map[string]string{
		"java.version":          "17",
		"maven.compiler.source": "${java.version}",
		"other.prop":            "value",
	}

	tests := []struct {
		name      string
		value     string
		expected  string
		expectErr bool
	}{
		{
			name:     "literal value",
			value:    "17",
			expected: "17",
		},
		{
			name:     "simple placeholder",
			value:    "${java.version}",
			expected: "17",
		},
		{
			name:      "unresolved placeholder",
			value:     "${undefined.property}",
			expectErr: true,
		},
		{
			name:     "nested placeholder",
			value:    "${maven.compiler.source}",
			expected: "17",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveValue(tt.value, props)

			if tt.expectErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestResolveValue_CircularReference(t *testing.T) {
	props := map[string]string{
		"a": "${b}",
		"b": "${a}",
	}

	_, err := resolveValue("${a}", props)
	if err == nil {
		t.Fatal("expected error for circular property reference, got nil")
	}

	var detErr *DetectionError
	if !errors.As(err, &detErr) {
		t.Fatalf("expected *DetectionError, got %T", err)
	}
	if !strings.Contains(detErr.Message, "circular") {
		t.Errorf("expected error message to mention 'circular', got %q", detErr.Message)
	}
}

func TestResolveValue_DeepChain(t *testing.T) {
	props := map[string]string{
		"a": "${b}",
		"b": "${c}",
		"c": "42",
	}

	result, err := resolveValue("${a}", props)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "42" {
		t.Errorf("expected %q, got %q", "42", result)
	}
}

func TestIsValidVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"11", true},
		{"17", true},
		{"21", true},
		{"8", true},
		{"", false},
		{"1.8", false},
		{"abc", false},
		{"17-ea", false},
		{"${java.version}", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isValidVersion(tt.input)
			if result != tt.expected {
				t.Errorf("isValidVersion(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsWithinWorkspace(t *testing.T) {
	tests := []struct {
		name      string
		workspace string
		path      string
		expected  bool
	}{
		{
			name:      "direct child",
			workspace: "/workspace",
			path:      "/workspace/pom.xml",
			expected:  true,
		},
		{
			name:      "nested child",
			workspace: "/workspace",
			path:      "/workspace/subdir/pom.xml",
			expected:  true,
		},
		{
			name:      "parent directory",
			workspace: "/workspace",
			path:      "/other/pom.xml",
			expected:  false,
		},
		{
			name:      "same directory",
			workspace: "/workspace",
			path:      "/workspace",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWithinWorkspace(tt.workspace, tt.path)
			if result != tt.expected {
				t.Errorf("isWithinWorkspace(%q, %q) = %v, expected %v",
					tt.workspace, tt.path, result, tt.expected)
			}
		})
	}
}
